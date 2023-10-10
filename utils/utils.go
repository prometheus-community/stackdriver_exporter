// Copyright 2020 The Prometheus Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package utils

import (
	"context"
	"regexp"
	"strings"

	"github.com/fatih/camelcase"
	"google.golang.org/api/cloudresourcemanager/v1"
)

var (
	safeNameRE = regexp.MustCompile(`[^a-zA-Z0-9_]*$`)
)

func NormalizeMetricName(metricName string) string {
	var normalizedMetricName []string

	words := camelcase.Split(metricName)
	for _, word := range words {
		safeWord := strings.Trim(safeNameRE.ReplaceAllLiteralString(word, "_"), "_")
		lowerWord := strings.TrimSpace(strings.ToLower(safeWord))
		if lowerWord != "" {
			normalizedMetricName = append(normalizedMetricName, lowerWord)
		}
	}

	return strings.Join(normalizedMetricName, "_")
}

func GetExtraFilterModifiers(extraFilter string, separator string) (string, string) {
	mPrefix := strings.Split(extraFilter, separator)
	if mPrefix[0] == extraFilter {
		return "", ""
	}
	return mPrefix[0], strings.Join(mPrefix[1:], "")
}

func ProjectResource(projectID string) string {
	return "projects/" + projectID
}

// GetProjectIDsFromQuery returns a list of project IDs from a Google Cloud organization using a query filter.
func GetProjectIDsFromQuery(ctx context.Context, query string) ([]string, error) {
	var projectIDs []string

	service, err := cloudresourcemanager.NewService(ctx)
	if err != nil {
		return nil, err
	}

	projects := service.Projects.List().Filter(query)
	if err := projects.Pages(context.Background(), func(page *cloudresourcemanager.ListProjectsResponse) error {
		for _, project := range page.Projects {
			projectIDs = append(projectIDs, project.ProjectId)
		}
		return nil
	}); err != nil {
		return nil, err
	}

	return projectIDs, nil
}
