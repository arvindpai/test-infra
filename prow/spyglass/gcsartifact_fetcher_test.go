/*
Copyright 2018 The Kubernetes Authors.

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

package spyglass

import (
	"encoding/base64"
	"fmt"
	"io/ioutil"
	"os"
	"strings"
	"testing"
)

func TestNewGCSJobSource(t *testing.T) {
	testCases := []struct {
		name        string
		src         string
		exJobPrefix string
		exBucket    string
		exName      string
		exBuildID   string
		expectedErr error
	}{
		{
			name:        "Test standard GCS link",
			src:         "test-bucket/logs/example-ci-run/403",
			exBucket:    "test-bucket",
			exJobPrefix: "logs/example-ci-run/403/",
			exName:      "example-ci-run",
			exBuildID:   "403",
			expectedErr: nil,
		},
		{
			name:        "Test GCS link with trailing /",
			src:         "test-bucket/logs/example-ci-run/403/",
			exBucket:    "test-bucket",
			exJobPrefix: "logs/example-ci-run/403/",
			exName:      "example-ci-run",
			exBuildID:   "403",
			expectedErr: nil,
		},
		{
			name:        "Test GCS link with org name",
			src:         "test-bucket/logs/sig-flexing/example-ci-run/403",
			exBucket:    "test-bucket",
			exJobPrefix: "logs/sig-flexing/example-ci-run/403/",
			exName:      "example-ci-run",
			exBuildID:   "403",
			expectedErr: nil,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			jobSource, err := newGCSJobSource(tc.src)
			if err != tc.expectedErr {
				t.Errorf("Expected err: %v, got err: %v", tc.expectedErr, err)
			}
			if tc.exBucket != jobSource.bucket {
				t.Errorf("Expected bucket %s, got %s", tc.exBucket, jobSource.bucket)
			}
			if tc.exName != jobSource.jobName {
				t.Errorf("Expected name %s, got %s", tc.exName, jobSource.jobName)
			}
			if tc.exJobPrefix != jobSource.jobPrefix {
				t.Errorf("Expected name %s, got %s", tc.exJobPrefix, jobSource.jobPrefix)
			}
		})
	}
}

// Tests listing objects associated with the current job in GCS
func TestArtifacts_ListGCS(t *testing.T) {
	fakeGCSClient := fakeGCSServer.Client()
	testAf := NewGCSArtifactFetcher(fakeGCSClient, "", false)
	testCases := []struct {
		name              string
		handle            artifactHandle
		source            string
		expectedArtifacts []string
	}{
		{
			name:   "Test ArtifactFetcher simple list artifacts",
			source: "test-bucket/logs/example-ci-run/403",
			expectedArtifacts: []string{
				"build-log.txt",
				"started.json",
				"finished.json",
				"junit_01.xml",
				"long-log.txt",
			},
		},
		{
			name:              "Test ArtifactFetcher list artifacts on source with no artifacts",
			source:            "test-bucket/logs/example-ci/404",
			expectedArtifacts: []string{},
		},
	}

	for _, tc := range testCases {
		actualArtifacts, err := testAf.artifacts(tc.source)
		if err != nil {
			t.Errorf("Failed to get artifact names: %v", err)
		}
		for _, ea := range tc.expectedArtifacts {
			found := false
			for _, aa := range actualArtifacts {
				if ea == aa {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("Case %s failed to retrieve the following artifact: %s\nRetrieved: %s.", tc.name, ea, actualArtifacts)
			}

		}
		if len(tc.expectedArtifacts) != len(actualArtifacts) {
			t.Errorf("Case %s produced more artifacts than expected. Expected: %s\nActual: %s.", tc.name, tc.expectedArtifacts, actualArtifacts)
		}
	}
}

// Tests getting handles to objects associated with the current job in GCS
func TestFetchArtifacts_GCS(t *testing.T) {
	fakeGCSClient := fakeGCSServer.Client()
	testAf := NewGCSArtifactFetcher(fakeGCSClient, "", false)
	maxSize := int64(500e6)
	testCases := []struct {
		name         string
		artifactName string
		source       string
		expectedSize int64
		expectErr    bool
	}{
		{
			name:         "Fetch build-log.txt from valid source",
			artifactName: "build-log.txt",
			source:       "test-bucket/logs/example-ci-run/403",
			expectedSize: 25,
		},
		{
			name:         "Fetch build-log.txt from invalid source",
			artifactName: "build-log.txt",
			source:       "test-bucket/logs/example-ci-run/404",
			expectErr:    true,
		},
	}

	for _, tc := range testCases {
		artifact, err := testAf.Artifact(tc.source, tc.artifactName, maxSize)
		if err != nil {
			t.Errorf("Failed to get artifacts: %v", err)
		}
		size, err := artifact.Size()
		if err != nil && !tc.expectErr {
			t.Fatalf("%s failed getting size for artifact %s, err: %v", tc.name, artifact.JobPath(), err)
		}
		if err == nil && tc.expectErr {
			t.Errorf("%s expected error, got no error", tc.name)
		}

		if size != tc.expectedSize {
			t.Errorf("%s expected artifact with size %d but got %d", tc.name, tc.expectedSize, size)
		}
	}
}

func TestSignURL(t *testing.T) {
	// This fake key is revoked and thus worthless but still make its contents less obvious
	fakeKeyBuf, err := base64.StdEncoding.DecodeString(`
LS0tLS1CRUdJTiBQUklWQVRFIEtFWS0tLS0tXG5NSUlFdlFJQkFEQU5CZ2txaGtpRzl3MEJBUUVG
QUFTQ0JLY3dnZ1NqQWdFQUFvSUJBUUN4MEF2aW1yMjcwZDdaXG5pamw3b1FRUW1oZTFOb3dpeWMy
UStuQW95aFE1YkQvUW1jb01zcWg2YldneVI0UU90aXVBbHM2VWhJenF4Q25pXG5PazRmbWJqVnhp
STl1Ri9EVTV6ZE5wM0dkQWFiUlVPNW5yWkpMelN0VXhudFBEcjZvK281RHM5YWJJWkNYYUVTXG5o
UWxOdTBrUm5HbHZGUHNkV1JYMmtSN01Yb3pkcXczcHZZRXZyaGlhRStYZnRhUzhKdmZEc0NPT2RQ
OWp5TzNTXG5aR2lkaU5hRmhYK2xnZEcrdHdqOUE3UDFlb1NMbTZCdXVhcjRDOGhlOEVkVGVEbXVk
a1BPeWwvb2tHWU5tSzJkXG5yUkQ0WHBhcy93VGxsTXBLRUZxWllZeVdkRnJvVWQwMFVhQnhHV0cz
UlZ2TWZoRk80QUhrSkNwZlE1U00rSElmXG5VN2lkRjAyYkFnTUJBQUVDZ2dFQURIaVhoTTZ1bFFB
OHZZdzB5T2Q3cGdCd3ZqeHpxckwxc0gvb0l1dzlhK09jXG5QREMxRzV2aU5pZjdRVitEc3haeXlh
T0tISitKVktQcWZodnh3OFNmMHBxQlowdkpwNlR6SVE3R0ZSZXBLUFc4XG5NTVloYWRPZVFiUE00
emN3dWNpS1VuTW45dU1hcllmc2xxUnZDUjBrSEZDWWtucHB2RjYxckNQMGdZZjJJRXZUXG5qNVlV
QWFrNDlVRDQyaUdEZnh2OGUzMGlMTmRRWE1iMHE3V2dyRGdxL0ttUHM2Q2dOaGRzME1uSlRFbUE5
YlFtXG52MHV0K2hUYWpXalcxVWNyUTBnM2JjNng1VWN2V1VjK1ZndUllVmxVcEgvM2dJNXVYZkxn
bTVQNThNa0s4UlhTXG5YYW92Rk05VkNNRFhTK25PWk1uSXoyNVd5QmhkNmdpVWs5UkJhc05Tb1FL
QmdRRGFxUXpyYWJUZEZNY1hwVlNnXG41TUpuNEcvSFVPWUxveVM5cE9UZi9qbFN1ZUYrNkt6RGJV
N1F6TC9wT1JtYjJldVdxdmpmZDVBaU1oUnY2Snk1XG41ZVNpa3dYRDZJeS9sZGh3QUdtMUZrZ1ZX
TXJ3ZHlqYjJpV2I2Um4rNXRBYjgwdzNEN2ZTWWhEWkxUOWJCNjdCXG4ybGxiOGFycEJRcndZUFFB
U2pUVUVYQnVJUUtCZ1FEUUxVemkrd0tHNEhLeko1NE1sQjFoR3cwSFZlWEV4T0pmXG53bS9IVjhl
aThDeHZLMTRoRXpCT3JXQi9aNlo4VFFxWnA0eENnYkNiY0hwY3pLRUxvcDA2K2hqa1N3ZkR2TUJZ
XG5mNnN6U2RSenNYVTI1NndmcG1hRjJ0TlJZZFpVblh2QWc5MFIrb1BFSjhrRHd4cjdiMGZmL3lu
b0UrWUx0ckowXG53dklad3Joc093S0JnQWVPbWlTMHRZeUNnRkwvNHNuZ3ZodEs5WElGQ0w1VU9C
dlp6Qk0xdlJOdjJ5eEFyRi9nXG5zajJqSmVyUWoyTUVpQkRmL2RQelZPYnBwaTByOCthMDNFOEdG
OGZxakpxK2VnbDg2aXBaQjhxOUU5NTFyOUxSXG5Xa1ZtTEFEVVIxTC8rSjFhakxiWHJzOWlzZkxh
ZEI2OUJpT1lXWmpPRk0reitocmNkYkR5blZraEFvR0FJbW42XG50ZU1zN2NNWTh3anZsY0MrZ3Br
SU5GZzgzYVIyajhJQzNIOWtYMGs0N3ovS0ZjbW9TTGxjcEhNc0VJeGozamJXXG5kd0FkZy9TNkpi
RW1SbGdoaWVoaVNRc21RM05ta0xxNlFJWkorcjR4VkZ4RUZnOWFEM0szVUZMT0xickRCSFpJXG5D
M3JRWVpMNkpnY1E1TlBtbTk4QXZIN2RucjRiRGpaVDgzSS9McFVDZ1lFQWttNXlvVUtZY0tXMVQz
R1hadUNIXG40SDNWVGVzZDZyb3pKWUhmTWVkNE9jQ3l1bnBIVmZmSmFCMFIxRjZ2MjFQaitCVWlW
WjBzU010RjEvTE1uQkc4XG5TQVlQUnVxOHVNUUdNQTFpdE1Hc2VhMmg1V2RhbXNGODhXRFd4VEoy
QXVnblJHNERsdmJLUDhPQmVLUFFKeDhEXG5RMzJ2SVpNUVkyV1hVMVhwUkMrNWs5RT1cbi0tLS0t
RU5EIFBSSVZBVEUgS0VZLS0tLS1cbgo=`)
	if err != nil {
		t.Fatalf("Failed to decode fake key: %v", err)
	}
	fakePrivateKey := strings.TrimSpace(string(fakeKeyBuf))
	cases := []struct {
		name      string
		fakeCreds string
		useCookie bool
		expected  string
		contains  []string
		err       bool
	}{
		{
			name:     "anon auth works",
			expected: fmt.Sprintf("https://%s/foo/bar/stuff", anonHost),
		},
		{
			name:      "cookie auth works",
			useCookie: true,
			expected:  fmt.Sprintf("https://%s/foo/bar/stuff", cookieHost),
		},
		{
			name:      "invalid json file errors",
			fakeCreds: "yaml: 123",
			err:       true,
		},
		{
			name: "bad private key errors",
			fakeCreds: `{
			  "type": "service_account",
			  "private_key": "-----BEGIN PRIVATE KEY-----\nMIIE==\n-----END PRIVATE KEY-----\n",
			  "client_email": "fake-user@k8s.io"
			}`,
			err: true,
		},
		{
			name: "bad type errors",
			fakeCreds: `{
			  "type": "user",
			  "private_key": "` + fakePrivateKey + `",
			  "client_email": "fake-user@k8s.io"
			}`,
			err: true,
		},
		{
			name: "signed URLs work",
			fakeCreds: `{
			  "type": "service_account",
			  "private_key": "` + fakePrivateKey + `",
			  "client_email": "fake-user@k8s.io"
			}`,
			contains: []string{
				"https://storage.googleapis.com/foo/bar/stuff?",
				"GoogleAccessId=fake-user%40k8s.io",
				"Signature=", // Do not particularly care about the Signature contents
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var path string
			if tc.fakeCreds != "" {
				fp, err := ioutil.TempFile("", "fake-creds")
				if err != nil {
					t.Fatalf("Failed to create fake creds: %v", err)
				}

				path = fp.Name()
				defer os.Remove(path)
				if _, err := fp.Write([]byte(tc.fakeCreds)); err != nil {
					t.Fatalf("Failed to write fake creds %s: %v", path, err)
				}

				if err := fp.Close(); err != nil {
					t.Fatalf("Failed to close fake creds %s: %v", path, err)
				}
			}
			af := NewGCSArtifactFetcher(nil, path, tc.useCookie)
			actual, err := af.signURL("foo", "bar/stuff")
			switch {
			case err != nil:
				if !tc.err {
					t.Errorf("unexpected error: %v", err)
				}
			case tc.err:
				t.Errorf("Failed to receive an expected error, got %q", actual)
			case len(tc.contains) == 0 && actual != tc.expected:
				t.Errorf("signURL(): got %q, want %q", actual, tc.expected)
			default:
				for _, part := range tc.contains {
					if !strings.Contains(actual, part) {
						t.Errorf("signURL(): got %q, does not contain %q", actual, part)
					}
				}
			}
		})
	}
}
