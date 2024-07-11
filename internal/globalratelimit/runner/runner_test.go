// Copyright Envoy Gateway Authors
// SPDX-License-Identifier: Apache-2.0
// The full text of the Apache license is available in the LICENSE file at
// the root of the repo.

package runner

import (
	"context"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/envoyproxy/gateway/internal/envoygateway/config"
	"github.com/envoyproxy/gateway/internal/infrastructure/kubernetes/ratelimit"
	"github.com/envoyproxy/gateway/internal/ir"
	"github.com/envoyproxy/gateway/internal/message"
	"github.com/envoyproxy/go-control-plane/pkg/cache/v3"
	cachev3 "github.com/envoyproxy/go-control-plane/pkg/cache/v3"
)

func Test_subscribeAndTranslate(t *testing.T) {
	t.Parallel()

	type xdsWithKey struct {
		key string
		xds *ir.Xds
	}

	testCases := []struct {
		name string
		// xdsIRs contains a list of xds resources to be Store()ed to the xdsIR.
		// Each element shows a pair of key and update that Runner will be get.
		xdsIRs       []xdsWithKey
		wantSnapshot cache.ResourceSnapshot
	}{
		{
			name: "one xds is added",
			xdsIRs: []xdsWithKey{
				{
					key: "gw1",
					xds: &ir.Xds{
						HTTP: []*ir.HTTPListener{
							{
								Routes: []*ir.HTTPRoute{
									{
										Traffic: &ir.TrafficFeatures{
											RateLimit: &ir.RateLimit{
												Global: &ir.GlobalRateLimit{
													Rules: []*ir.RateLimitRule{
														{
															HeaderMatches: []*ir.StringMatch{
																{
																	Name:     "x-user-id",
																	Distinct: true,
																},
															},
														},
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			wantSnapshot: &cache.Snapshot{},
		},
	}

	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			xdsIR := new(message.XdsIR)
			defer xdsIR.Close()
			cfg, err := config.New()
			require.NoError(t, err)

			r := New(&Config{
				Server: *cfg,
				XdsIR:  xdsIR,
				cache:  cachev3.NewSnapshotCache(false, cachev3.IDHash{}, nil),
			})

			go r.subscribeAndTranslate(ctx)

			for _, xds := range tt.xdsIRs {
				xdsIR.Store(xds.key, xds.xds)
			}

			diff := ""
			if !assert.Eventually(t, func() bool {
				s, err := r.cache.GetSnapshot(ratelimit.InfraName)
				if err != nil {
					t.Fatalf("failed to get snapshot: %v", err)
					return false
				}

				diff = cmp.Diff(tt.wantSnapshot, s)
				return diff == ""
			}, time.Second*1, time.Millisecond*20) {
				t.Fatalf("snapshot mismatch (-want +got):\n%s", diff)
			}
		})
	}

}
