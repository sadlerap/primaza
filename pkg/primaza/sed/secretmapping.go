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

// Package sed contains logic for ServiceEndpointDefinition
package sed

import (
	"context"
	"fmt"

	"github.com/primaza/primaza/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/jsonpath"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type SEDSecretRefMapping struct {
	namespace string
	resource  unstructured.Unstructured
	cli       client.Client

	key        string
	secretName *jsonpath.JSONPath
	secretKey  *jsonpath.JSONPath
}

func NewSEDSecretRefMapping(
	namespace string,
	resource unstructured.Unstructured,
	cli client.Client,
	mapping v1alpha1.ServiceClassSecretRefFieldMapping,
) (*SEDSecretRefMapping, error) {
	pathName := jsonpath.New("")
	if err := pathName.Parse(fmt.Sprintf("{%s}", mapping.SecretName)); err != nil {
		return nil, err
	}

	pathKey := jsonpath.New("")
	if err := pathKey.Parse(fmt.Sprintf("{%s}", mapping.SecretKey)); err != nil {
		return nil, err
	}

	return &SEDSecretRefMapping{
		namespace:  namespace,
		resource:   resource,
		cli:        cli,
		key:        mapping.Name,
		secretKey:  pathKey,
		secretName: pathName,
	}, nil
}

func (s *SEDSecretRefMapping) Key() string {
	return s.key
}

func (mapping *SEDSecretRefMapping) ReadKey(ctx context.Context) (*string, error) {
	secKey, err := readSingleJsonPath(mapping.secretKey, mapping.resource)
	if err != nil {
		return nil, err
	}
	secName, err := readSingleJsonPath(mapping.secretName, mapping.resource)
	if err != nil {
		return nil, err
	}

	s := &corev1.Secret{}
	ok := types.NamespacedName{
		Namespace: mapping.namespace,
		Name:      *secName,
	}
	if err := mapping.cli.Get(ctx, ok, s, &client.GetOptions{}); err != nil {
		return nil, err
	}

	if vb, ok := s.Data[*secKey]; ok {
		v := string(vb)
		return &v, nil
	}

	return nil, fmt.Errorf("secret key '%s/%s:%s' not Found", mapping.namespace, *secName, *secKey)
}

func readSingleJsonPath(path *jsonpath.JSONPath, resource unstructured.Unstructured) (*string, error) {
	results, err := path.FindResults(resource.Object)
	if err != nil {
		return nil, err
	}

	if len(results) != 1 || len(results[0]) != 1 {
		return nil, fmt.Errorf("jsonPath lookup into resource returned multiple results: %v", results)
	}

	value := fmt.Sprintf("%v", results[0][0])
	return &value, nil
}

func (s *SEDSecretRefMapping) InSecret() bool {
	return true
}
