// Copyright (c) 2020, 2022, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

package ndbconfig

import (
	"bytes"
	"text/template"

	"github.com/mysql/ndb-operator/pkg/apis/ndbcontroller/v1alpha1"
	"github.com/mysql/ndb-operator/pkg/constants"
)

// MySQL Cluster config template
var mgmtConfigTmpl = `{{- /* Template to generate management config ini */ -}}
# Auto generated config.ini - DO NOT EDIT

[system]
ConfigGenerationNumber={{GetConfigVersion}}
Name={{.Name}}

[ndbd default]
NoOfReplicas={{.Spec.RedundancyLevel}}
DataMemory={{.Spec.DataMemory}}
# Use a fixed ServerPort for all data nodes
ServerPort=1186

[tcp default]
AllowUnresolvedHostnames=1

{{range $idx, $nodeId := GetNodeIds "mgmd" -}}
[ndb_mgmd]
NodeId={{$nodeId}}
{{/*TODO - get and use the real domain name instead of .svc.cluster.local */ -}}
Hostname={{$.Name}}-mgmd-{{$idx}}.{{$.GetServiceName "mgmd"}}.{{$.Namespace}}.svc.cluster.local
DataDir={{GetDataDir}}

{{end -}}
{{range $idx, $nodeId := GetNodeIds "ndbd" -}}
[ndbd]
NodeId={{$nodeId}}
{{/*TODO - get and use the real domain name instead of .svc.cluster.local */ -}}
Hostname={{$.Name}}-ndbd-{{$idx}}.{{$.GetServiceName "ndbd"}}.{{$.Namespace}}.svc.cluster.local
DataDir={{GetDataDir}}

{{end -}}
{{range $nodeId := GetNodeIds "api" -}}
[api]
NodeId={{$nodeId}}

{{end -}}
`

// GetConfigString generates a new configuration for the
// MySQL Cluster from the given ndb resources Spec.
//
// It is important to note that GetConfigString uses the Spec in its
// actual and consistent state and does not rely on any Status field.
func GetConfigString(ndb *v1alpha1.NdbCluster, oldConfigSummary *ConfigSummary) (string, error) {

	var (
		// Variable that keeps track of the first free data node, mgmd node ids
		ndbdMgmdStartNodeId = 1
		// Right now maximum of 144 data nodes are allowed in a cluster
		// So to accommodate scaling up of those nodes, start the api node id from 145
		// TODO: Validate the maximum number of API ids
		apiStartNodeId = 145
	)

	tmpl := template.New("config.ini")
	tmpl.Funcs(template.FuncMap{
		// GetNodeIds returns an array of node ids for the given node type
		"GetNodeIds": func(nodeType string) []int {
			var startNodeId *int
			var numberOfNodes int32
			switch nodeType {
			case "mgmd":
				startNodeId = &ndbdMgmdStartNodeId
				numberOfNodes = ndb.GetManagementNodeCount()
			case "ndbd":
				startNodeId = &ndbdMgmdStartNodeId
				numberOfNodes = ndb.Spec.NodeCount
			case "api":
				startNodeId = &apiStartNodeId
				// Calculate the total number of API slots to be set in the config.
				// slots required for mysql servers + free NDBAPI slots.
				numberOfNodes = getNumOfRequiredAPISections(ndb, oldConfigSummary) + ndb.Spec.FreeAPISlots
			default:
				panic("Unrecognised node type")
			}
			// generate nodeIds based on start node id and number of nodes
			nodeIds := make([]int, numberOfNodes)
			for i := range nodeIds {
				nodeIds[i] = *startNodeId
				*startNodeId++
			}
			return nodeIds
		},
		"GetDataDir": func() string { return constants.DataDir },
		"GetConfigVersion": func() int32 {
			if oldConfigSummary == nil {
				// First version of the management config based on newly added NdbCluster spec.
				return 1
			} else {
				// Bump up the config version for every change.
				return oldConfigSummary.MySQLClusterConfigVersion + 1
			}
		},
	})

	if _, err := tmpl.Parse(mgmtConfigTmpl); err != nil {
		// panic to discover any parsing errors during development
		panic("Failed to parse mgmt config template")
	}

	var configIni bytes.Buffer
	if err := tmpl.Execute(&configIni, ndb); err != nil {
		return "", err
	}

	return configIni.String(), nil
}
