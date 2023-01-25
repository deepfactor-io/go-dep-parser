package utils

import (
	"fmt"
	"net/http"

	"github.com/deepfactor-io/go-dep-parser/pkg/types"
)

const JAVA_ARTIFACT_PARSER_ERROR = "JAVA_ARTIFACT_PARSER_ERROR:"

func IsRetryableError(statusCode int) bool {
	if statusCode == http.StatusTooManyRequests {
		return true
	}

	if statusCode == 0 || (statusCode >= 500 && statusCode != http.StatusNotImplemented) {
		return true
	}

	return false
}

func UniqueStrings(ss []string) []string {
	var results []string
	uniq := map[string]struct{}{}
	for _, s := range ss {
		if _, ok := uniq[s]; ok {
			continue
		}
		results = append(results, s)
		uniq[s] = struct{}{}
	}
	return results
}

func UniqueLibraries(libs []types.Library) []types.Library {
	var uniqLibs []types.Library
	unique := map[string]struct{}{}
	for _, lib := range libs {
		identifier := fmt.Sprintf("%s@%s", lib.Name, lib.Version)
		if _, ok := unique[identifier]; !ok {
			unique[identifier] = struct{}{}
			uniqLibs = append(uniqLibs, lib)
		}
	}
	return uniqLibs
}

func MergeMaps(parent, child map[string]string) map[string]string {
	if parent == nil {
		return child
	}
	for k, v := range child {
		parent[k] = v
	}
	return parent
}

func PackageID(name, version string) string {
	return fmt.Sprintf("%s@%s", name, version)
}
