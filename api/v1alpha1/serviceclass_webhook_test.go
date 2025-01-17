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

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func newServiceClass(name, namespace string, spec ServiceClassSpec) ServiceClass {
	return ServiceClass{
		ObjectMeta: v1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: spec,
	}
}

var _ = Describe("Webhook tests", func() {
	var validator serviceClassValidator
	BeforeEach(func() {
		schemeBuilder, err := SchemeBuilder.Build()
		Expect(err).NotTo(HaveOccurred())

		validator = serviceClassValidator{
			client: fake.NewClientBuilder().
				WithScheme(schemeBuilder).
				WithLists(&ServiceClassList{}).
				Build(),
		}
	})
	DescribeTable("Creation validation failures",
		func(serviceClass ServiceClass, expected error) {
			Expect(validator.ValidateCreate(context.Background(), &serviceClass)).To(Equal(expected))
		},
		Entry("Invalid jsonpaths",
			newServiceClass("spam", "eggs",
				ServiceClassSpec{
					Resource: ServiceClassResource{
						APIVersion: "foo.bar/v1",
						Kind:       "baz",
						ServiceEndpointDefinitionMappings: ServiceEndpointDefinitionMappings{
							ResourceFields: []ServiceClassResourceFieldMapping{
								{
									Name:     "x",
									JsonPath: ".invalid[*",
								},
							},
						},
					},
				},
			),
			field.ErrorList{
				field.Invalid(field.NewPath("spec", "resource", "serviceEndpointDefinitionMapping").Index(0).Child("jsonPath"), ".invalid[*", "Invalid JSONPath"),
			}.ToAggregate()),
		Entry("Duplicate names",
			newServiceClass("spam", "eggs",
				ServiceClassSpec{
					Resource: ServiceClassResource{
						APIVersion: "foo.bar/v1",
						Kind:       "baz",
						ServiceEndpointDefinitionMappings: ServiceEndpointDefinitionMappings{
							ResourceFields: []ServiceClassResourceFieldMapping{
								{
									Name:     "x",
									JsonPath: ".spec",
								},
								{
									Name:     "x",
									JsonPath: ".metadata",
								},
							},
						},
					},
				},
			),
			field.ErrorList{
				field.Duplicate(field.NewPath("spec", "resource", "serviceEndpointDefinitionMapping").Index(1).Child("name"), "x"),
			}.ToAggregate()),
	)

	DescribeTable("Update validation failures",
		func(oldClass, newClass ServiceClass, expected error) {
			Expect(validator.ValidateUpdate(context.Background(), &oldClass, &newClass)).To(Equal(expected))
		},
		Entry("Resource Kind is immutable",
			newServiceClass("spam", "eggs",
				ServiceClassSpec{
					Resource: ServiceClassResource{
						APIVersion: "foo.bar/v1",
						Kind:       "baz",
						ServiceEndpointDefinitionMappings: ServiceEndpointDefinitionMappings{
							ResourceFields: []ServiceClassResourceFieldMapping{
								{
									Name:     "x",
									JsonPath: ".spec",
								},
							},
						},
					},
				}),
			newServiceClass("spam", "eggs",
				ServiceClassSpec{
					Resource: ServiceClassResource{
						APIVersion: "foo.bar/v1",
						Kind:       "bam",
						ServiceEndpointDefinitionMappings: ServiceEndpointDefinitionMappings{
							ResourceFields: []ServiceClassResourceFieldMapping{
								{
									Name:     "x",
									JsonPath: ".spec",
								},
							},
						},
					},
				}),
			field.ErrorList{
				field.Invalid(field.NewPath("spec", "resource", "kind"), "bam", "Kind is immutable"),
			}.ToAggregate()),
		Entry("Resource APIVersion is immutable",
			newServiceClass("spam", "eggs",
				ServiceClassSpec{
					Resource: ServiceClassResource{
						APIVersion: "foo.bar/v1",
						Kind:       "baz",
						ServiceEndpointDefinitionMappings: ServiceEndpointDefinitionMappings{
							ResourceFields: []ServiceClassResourceFieldMapping{
								{
									Name:     "x",
									JsonPath: ".spec",
								},
							},
						},
					},
				}),
			newServiceClass("spam", "eggs",
				ServiceClassSpec{
					Resource: ServiceClassResource{
						APIVersion: "foo.bam",
						Kind:       "baz",
						ServiceEndpointDefinitionMappings: ServiceEndpointDefinitionMappings{
							ResourceFields: []ServiceClassResourceFieldMapping{
								{
									Name:     "x",
									JsonPath: ".spec",
								},
							},
						},
					},
				}),
			field.ErrorList{
				field.Invalid(field.NewPath("spec", "resource", "apiVersion"), "foo.bam", "APIVersion is immutable"),
			}.ToAggregate()),
		Entry("Resource APIVersion is immutable",
			newServiceClass("spam", "eggs",
				ServiceClassSpec{
					Resource: ServiceClassResource{
						APIVersion: "foo.bar/v1",
						Kind:       "baz",
						ServiceEndpointDefinitionMappings: ServiceEndpointDefinitionMappings{
							ResourceFields: []ServiceClassResourceFieldMapping{
								{
									Name:     "x",
									JsonPath: ".spec",
								},
							},
						},
					},
				}),
			newServiceClass("spam", "eggs",
				ServiceClassSpec{
					Resource: ServiceClassResource{
						APIVersion: "foo.bar/v1",
						Kind:       "baz",
						ServiceEndpointDefinitionMappings: ServiceEndpointDefinitionMappings{
							ResourceFields: []ServiceClassResourceFieldMapping{
								{
									Name:     "x",
									JsonPath: ".metadata",
								},
							},
						},
					},
				}),
			field.ErrorList{
				field.Invalid(
					field.NewPath("spec", "resource", "serviceEndpointDefinitionMapping"),
					ServiceEndpointDefinitionMappings{
						ResourceFields: []ServiceClassResourceFieldMapping{
							{
								Name:     "x",
								JsonPath: ".metadata",
							},
						},
					},
					"ServiceEndpointDefinitionMapping is immutable"),
			}.ToAggregate()),
	)

	It("should reject non-ServiceClass old objects", func() {
		oldObject := unstructured.Unstructured{}
		newObject := newServiceClass("spam", "eggs", ServiceClassSpec{
			Resource: ServiceClassResource{
				APIVersion: "foo.bar/v1",
				Kind:       "baz",
				ServiceEndpointDefinitionMappings: ServiceEndpointDefinitionMappings{
					ResourceFields: []ServiceClassResourceFieldMapping{
						{
							Name:     "x",
							JsonPath: ".metadata",
						},
					},
				},
			},
		})
		Expect(validator.ValidateCreate(context.Background(), &oldObject)).To(HaveOccurred())
		Expect(validator.ValidateUpdate(context.Background(), &oldObject, &newObject)).To(HaveOccurred())
		Expect(validator.ValidateUpdate(context.Background(), &newObject, &oldObject)).To(HaveOccurred())
		Expect(validator.ValidateDelete(context.Background(), &oldObject)).To(HaveOccurred())
	})

	It("should allow delete requests", func() {
		object := newServiceClass("spam", "eggs", ServiceClassSpec{
			Resource: ServiceClassResource{
				APIVersion: "foo.bar/v1",
				Kind:       "baz",
				ServiceEndpointDefinitionMappings: ServiceEndpointDefinitionMappings{
					ResourceFields: []ServiceClassResourceFieldMapping{
						{
							Name:     "x",
							JsonPath: ".metadata",
						},
					},
				},
			},
		})
		Expect(validator.ValidateDelete(context.Background(), &object)).NotTo(HaveOccurred())
	})

	It("should disallow service classes with the same resource type", func() {
		schemeBuilder, err := SchemeBuilder.Build()
		Expect(err).NotTo(HaveOccurred())

		class := newServiceClass("spam", "eggs", ServiceClassSpec{
			Resource: ServiceClassResource{
				APIVersion: "foo.bar/v1",
				Kind:       "baz",
				ServiceEndpointDefinitionMappings: ServiceEndpointDefinitionMappings{
					ResourceFields: []ServiceClassResourceFieldMapping{
						{
							Name:     "x",
							JsonPath: ".metadata",
						},
					},
				},
			},
		})

		validator = serviceClassValidator{
			client: fake.NewClientBuilder().
				WithScheme(schemeBuilder).
				WithLists(&ServiceClassList{}).
				WithRuntimeObjects(class.DeepCopy()).
				Build(),
		}
		class.Name = "beans"

		Expect(validator.ValidateCreate(context.Background(), &class)).To(Equal(
			field.ErrorList{
				field.Forbidden(field.NewPath("spec", "resource"), "Service Class spam already manages services of type baz.foo.bar/v1"),
			}.ToAggregate(),
		))

		Expect(validator.ValidateUpdate(context.Background(), &class, &class)).To(Equal(
			field.ErrorList{
				field.Forbidden(field.NewPath("spec", "resource"), "Service Class spam already manages services of type baz.foo.bar/v1"),
			}.ToAggregate(),
		))
	})
})
