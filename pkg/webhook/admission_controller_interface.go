// Copyright (c) 2021, 2022, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

package webhook

import (
	"fmt"
	"regexp"

	admissionv1 "k8s.io/api/admission/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/klog/v2"
)

type admissionController interface {
	getGVR() *metav1.GroupVersionResource
	getGVK() *schema.GroupVersionKind
	newObject() runtime.Object
	// validate functions should validate the request and return a AdmissionResponse
	validateCreate(reqUID types.UID, obj runtime.Object) *admissionv1.AdmissionResponse
	validateUpdate(reqUID types.UID, obj runtime.Object, oldObj runtime.Object) *admissionv1.AdmissionResponse
	// mutate function should return the JSONPatch that needs to be applied to the resource
	mutate(obj runtime.Object) ([]byte, error)
}

func unsupportedValidatorOperation(reqUID types.UID, operation admissionv1.Operation) *admissionv1.AdmissionResponse {
	errMsg := fmt.Sprintf("validating a %s operation not supported", operation)
	klog.Error(errMsg)
	return requestDeniedBad(reqUID, errMsg)
}

type requestExecutor func(req *admissionv1.AdmissionRequest, ac admissionController) *admissionv1.AdmissionResponse

func validate(req *admissionv1.AdmissionRequest, ac admissionController) *admissionv1.AdmissionResponse {
	// Verify right resource is passed
	resource := ac.getGVR()
	if req.Resource != *resource {
		errMsg := fmt.Sprintf("expected resource %v but got %v", *resource, req.Resource)
		return requestDeniedBad(req.UID, errMsg)
	}

	// Handle operation
	defaultGVK := ac.getGVK()
	decoder := scheme.Codecs.UniversalDeserializer()
	switch req.Operation {
	case admissionv1.Create:
		// retrieve new object and validate it
		obj, _, err := decoder.Decode(req.Object.Raw, defaultGVK, ac.newObject())
		if err != nil {
			return requestDeniedBad(req.UID, err.Error())
		}
		klog.V(5).Info(fmt.Sprintf("Retrieved new object : %v", obj))
		return ac.validateCreate(req.UID, obj)

	case admissionv1.Update:
		// any updates made from the ndb-operator can be accepted without validation
		if updateFromNdbOperator, _ := regexp.MatchString(
			"system:serviceaccount:.*:ndb-operator", req.UserInfo.Username); updateFromNdbOperator {
			klog.Info("Skipping validation for an update from ndb-operator")
			return requestAllowed(req.UID)
		}

		// retrieve new and old objects
		obj, _, err := decoder.Decode(req.Object.Raw, defaultGVK, ac.newObject())
		if err != nil {
			return requestDeniedBad(req.UID, err.Error())
		}
		klog.V(5).Info(fmt.Sprintf("Retrieved new object : %v", obj))

		oldObject, _, err := decoder.Decode(req.OldObject.Raw, defaultGVK, ac.newObject())
		if err != nil {
			return requestDeniedBad(req.UID, err.Error())
		}
		klog.V(5).Info(fmt.Sprintf("Retrieved old object : %v", oldObject))

		// validate the update
		return ac.validateUpdate(req.UID, obj, oldObject)

	default:
		return unsupportedValidatorOperation(req.UID, req.Operation)
	}
}

func mutate(req *admissionv1.AdmissionRequest, ac admissionController) *admissionv1.AdmissionResponse {
	// Verify right resource is passed
	resource := ac.getGVR()
	if req.Resource != *resource {
		errMsg := fmt.Sprintf("expected resource %v but got %v", *resource, req.Resource)
		return requestDeniedBad(req.UID, errMsg)
	}

	// Decode the object and mutate
	defaultGVK := ac.getGVK()
	decoder := scheme.Codecs.UniversalDeserializer()

	// retrieve new object and mutate it
	obj, _, err := decoder.Decode(req.Object.Raw, defaultGVK, ac.newObject())
	if err != nil {
		return requestDeniedBad(req.UID, err.Error())
	}

	patch, err := ac.mutate(obj)
	if err != nil {
		return requestDeniedBad(req.UID, err.Error())
	}

	// Request allowed
	admissionResponse := &admissionv1.AdmissionResponse{
		UID:     req.UID,
		Allowed: true,
	}

	// Update the response with the patch if required
	if len(patch) > 0 {
		admissionResponse.Patch = patch
		patchType := admissionv1.PatchTypeJSONPatch
		admissionResponse.PatchType = &patchType
	}

	return admissionResponse
}
