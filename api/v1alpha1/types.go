/*
Copyright 2023 The Primaza Authors.

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

package v1alpha1

// ServiceClassIdentityItem defines an attribute that is necessary to
// identify a service class.
type ServiceClassIdentityItem struct {
	// Name of the service class identity attribute.
	Name string `json:"name"`

	// Value of the service class identity attribute.
	Value string `json:"value"`
}

// HealthCheckContainer defines the container information to be used to
// run helth checks for the service.
type HealthCheckContainer struct {
	// Container image with the client to run the test
	Image string `json:"image"`
	// Command to execute in the container to run the test
	Command string `json:"command"`
}

// HealthCheck defines metadata that can be used check
// the health of a service and report status.
type HealthCheck struct {
	// Container defines a container that will run a check against the
	// ServiceEndpointDefinition to determine connectivity and access.
	Container HealthCheckContainer `json:"container"`
}
