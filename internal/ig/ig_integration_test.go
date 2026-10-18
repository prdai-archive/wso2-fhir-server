//go:build integration

// Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
//
// WSO2 LLC. licenses this file to you under the Apache License,
// Version 2.0 (the "License"); you may not use this file except
// in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package ig

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/wso2/fhir-server/internal/testutil"
)

func TestLoadPackage_RefreshesRegistryAfterSameSourceConflict(t *testing.T) {
	pool := testutil.MustSeededDB(t)
	packageData := buildTestTgz(t, map[string][]byte{
		"package/package.json": mustJSON(map[string]any{
			"name":        "test.ig",
			"version":     "1.0.0",
			"fhirVersion": "4.0.1",
		}),
		"package/SearchParameter-test.json": mustJSON(map[string]any{
			"resourceType": "SearchParameter",
			"code":         "ig-race-test",
			"type":         "string",
			"base":         []string{"Patient"},
			"expression":   "Patient.name.given",
		}),
		"package/SearchParameter-active.json": mustJSON(map[string]any{
			"resourceType": "SearchParameter",
			"code":         "active",
			"type":         "token",
			"base":         []string{"Patient"},
			"expression":   "Patient.name",
		}),
	})
	registryServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(packageData)
	}))
	defer registryServer.Close()

	firstRegistry := testutil.MustRegistry(t, pool)
	secondRegistry := testutil.MustRegistry(t, pool)
	ctx := context.Background()
	spec := "test.ig@1.0.0"
	opts := LoadOptions{RegistryURL: registryServer.URL}
	if _, err := LoadPackage(ctx, pool, firstRegistry, spec, opts); err != nil {
		t.Fatalf("load package on first replica: %v", err)
	}

	opts.ForceReload = true
	result, err := LoadPackage(ctx, pool, secondRegistry, spec, opts)
	if err != nil {
		t.Fatalf("load package on second replica: %v", err)
	}
	if result.SearchParams != 0 {
		t.Fatalf("expected no new rows on duplicate load, got %d", result.SearchParams)
	}
	if _, ok := secondRegistry.Lookup("Patient", "ig-race-test"); !ok {
		t.Fatal("second replica registry did not load the persisted same-source SearchParameter")
	}
	if def, ok := secondRegistry.Lookup("Patient", "active"); !ok || def.FHIRPath != "Patient.active" {
		t.Fatalf("IG conflict replaced the base definition: %+v, found=%v", def, ok)
	}
}
