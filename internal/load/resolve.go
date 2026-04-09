package load

import (
	"strings"

	awsclient "github.com/rmukubvu/preflight/pkg/aws"
)

func resolveAPIRef(resources []awsclient.StackResource, apis []awsclient.APIDetail, ref string) string {
	if resolved := resolvePhysicalResourceID(resources, "AWS::ApiGatewayV2::Api", ref); resolved != ref {
		for _, api := range apis {
			if api.APIID == resolved || api.Name == resolved {
				return api.APIID
			}
		}
	}

	ref = strings.TrimSpace(ref)
	for _, api := range apis {
		if api.APIID == ref || api.Name == ref || strings.HasPrefix(api.Name, ref) {
			return api.APIID
		}
	}
	return ref
}

func resolveLambdaRef(resources []awsclient.StackResource, ref string) string {
	return resolvePhysicalResourceID(resources, "AWS::Lambda::Function", ref)
}

func resolvePhysicalResourceID(resources []awsclient.StackResource, resourceType, ref string) string {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return ""
	}
	for _, resource := range resources {
		if resource.Type != resourceType {
			continue
		}
		if resource.LogicalID == ref || resource.PhysicalID == ref {
			return resource.PhysicalID
		}
		if strings.HasPrefix(resource.LogicalID, ref) {
			return resource.PhysicalID
		}
	}
	return ref
}
