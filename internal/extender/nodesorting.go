// Copyright (c) 2019 Palantir Technologies. All rights reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package extender

import (
	"sort"

	"github.com/palantir/k8s-spark-scheduler-lib/pkg/resources"
)

type scheduleContext struct {
	// Lower value of priority indicates that the AZ has less resources
	azPriority    int
	nodeResources *resources.Resources
}

// Sort by available resources ascending, with RAM usage more important.
func resourcesLessThan(left *resources.Resources, right *resources.Resources) bool {
	var memoryCompared = left.Memory.Cmp(right.Memory)
	if memoryCompared != 0 {
		return memoryCompared == -1
	}
	return left.CPU.Cmp(right.CPU) == -1
}

// Sort first by AZ priority and then by resources on the node
func scheduleContextLessThan(left scheduleContext, right scheduleContext) bool {
	if left.azPriority != right.azPriority {
		return left.azPriority < right.azPriority
	}
	return resourcesLessThan(left.nodeResources, right.nodeResources)
}

func getNodeNamesInPriorityOrder(useExperimentalHostPriorities bool, nodesSchedulingMetadata resources.NodeGroupSchedulingMetadata) []string {
	nodeNames := getNodeNames(nodesSchedulingMetadata)
	if !useExperimentalHostPriorities {
		sort.Slice(nodeNames, func(i, j int) bool {
			return nodesSchedulingMetadata[nodeNames[j]].CreationTimestamp.Before(nodesSchedulingMetadata[nodeNames[i]].CreationTimestamp)
		})
		return nodeNames
	}

	var nodeNamesByAZ = groupNodeNamesByAZ(nodesSchedulingMetadata)
	var allAzLabels = getAllAZLabels(nodeNamesByAZ)
	var availableResourcesByAZ = getAvailableResourcesByAZ(nodeNamesByAZ, nodesSchedulingMetadata)

	sort.Slice(allAzLabels, func(i, j int) bool {
		return resourcesLessThan(availableResourcesByAZ[allAzLabels[i]], availableResourcesByAZ[allAzLabels[j]])
	})

	var scheduleContexts = make(map[string]scheduleContext, len(nodeNames))
	for azPriority, azLabel := range allAzLabels {
		for _, nodeName := range nodeNamesByAZ[azLabel] {
			scheduleContexts[nodeName] = scheduleContext{
				azPriority,
				nodesSchedulingMetadata[nodeName].AvailableResources,
			}
		}
	}

	sort.Slice(nodeNames, func(i, j int) bool {
		return scheduleContextLessThan(scheduleContexts[nodeNames[i]], scheduleContexts[nodeNames[j]])
	})

	return nodeNames
}

func getAvailableResourcesByAZ(nodesByAZ map[string][]string, nodesSchedulingMetadata resources.NodeGroupSchedulingMetadata) map[string]*resources.Resources {
	var availableResourcesByAZ = make(map[string]*resources.Resources, len(nodesByAZ))
	for azLabel, nodesInAz := range nodesByAZ {
		var azResources = resources.Zero()
		for _, nodeName := range nodesInAz {
			azResources.Add(nodesSchedulingMetadata[nodeName].AvailableResources)
		}
		availableResourcesByAZ[azLabel] = azResources
	}
	return availableResourcesByAZ
}

func groupNodeNamesByAZ(nodesSchedulingMetadata resources.NodeGroupSchedulingMetadata) map[string][]string {
	nodesByAZ := make(map[string][]string)
	for nodeName, nodeSchedulingMetadata := range nodesSchedulingMetadata {
		azLabel := nodeSchedulingMetadata.ZoneLabel
		nodesByAZ[azLabel] = append(nodesByAZ[azLabel], nodeName)
	}
	return nodesByAZ
}

func getAllAZLabels(nodesByAZ map[string][]string) []string {
	azLabels := make([]string, 0)
	for key := range nodesByAZ {
		azLabels = append(azLabels, key)
	}
	return azLabels
}

func getNodeNames(nodesSchedulingMetadata resources.NodeGroupSchedulingMetadata) []string {
	nodeNames := make([]string, 0, len(nodesSchedulingMetadata))
	for key := range nodesSchedulingMetadata {
		nodeNames = append(nodeNames, key)
	}
	return nodeNames
}
