/*

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controllers

import (
	dspav1alpha1 "github.com/opendatahub-io/data-science-pipelines-operator/api/v1alpha1"
)

var commonTemplates = []string{
	"common/policy.yaml.tmpl",
	"common/mlmd-envoy-dashboard-access-policy.yaml.tmpl",
}

const commonCusterRolebindingTemplate = "common/clusterrolebinding.yaml.tmpl"

func (r *DSPAReconciler) ReconcileCommon(dsp *dspav1alpha1.DataSciencePipelinesApplication, params *DSPAParams) error {
	log := r.Log.WithValues("namespace", dsp.Namespace).WithValues("dspa_name", dsp.Name)

	log.Info("Applying Common Resources")
	for _, template := range commonTemplates {
		err := r.Apply(dsp, params, template)
		if err != nil {
			return err
		}
	}

	err := r.ApplyWithoutOwner(params, commonCusterRolebindingTemplate)
	if err != nil {
		return err
	}

	log.Info("Finished applying Common Resources")
	return nil
}

func (r *DSPAReconciler) CleanUpCommon(params *DSPAParams) error {
	err := r.DeleteResource(params, commonCusterRolebindingTemplate)
	if err != nil {
		return err
	}
	return nil
}
