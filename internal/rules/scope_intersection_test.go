package rules

import (
	"testing"

	"github.com/octopusdeploy/octopus-permissions-controller/api/v1beta1"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func Test_getScopes(t *testing.T) {
	type args struct {
		wsaList []v1beta1.WorkloadServiceAccount
	}
	tests := []struct {
		name string
		args args
		want []string
	}{
		{
			name: "Single WSA, single scope",
			args: args{
				wsaList: []v1beta1.WorkloadServiceAccount{
					{
						ObjectMeta: metav1.ObjectMeta{Name: "wsa1"},
						Spec: v1beta1.WorkloadServiceAccountSpec{
							Scope: v1beta1.WorkloadServiceAccountScope{
								Projects: []string{"proj1"},
							},
						},
					},
				},
			},
			want: []string{"wsa1"},
		},
		{
			name: "Multiple WSA with no intersection, two scopes",
			args: args{
				wsaList: []v1beta1.WorkloadServiceAccount{
					{
						ObjectMeta: metav1.ObjectMeta{Name: "wsa1"},
						Spec: v1beta1.WorkloadServiceAccountSpec{
							Scope: v1beta1.WorkloadServiceAccountScope{
								Projects: []string{"proj1"},
							},
						},
					},
					{
						ObjectMeta: metav1.ObjectMeta{Name: "wsa2"},
						Spec: v1beta1.WorkloadServiceAccountSpec{
							Scope: v1beta1.WorkloadServiceAccountScope{
								Projects: []string{"proj2"},
							},
						},
					},
				},
			},
			want: []string{"wsa1", "wsa2"},
		},
		{
			name: "Multiple WSA with intersection, three scopes",
			args: args{
				wsaList: []v1beta1.WorkloadServiceAccount{
					{
						ObjectMeta: metav1.ObjectMeta{Name: "wsa1"},
						Spec: v1beta1.WorkloadServiceAccountSpec{
							Scope: v1beta1.WorkloadServiceAccountScope{
								Environments: []string{"env1"},
							},
						},
					},
					{
						ObjectMeta: metav1.ObjectMeta{Name: "wsa2"},
						Spec: v1beta1.WorkloadServiceAccountSpec{
							Scope: v1beta1.WorkloadServiceAccountScope{
								Projects: []string{"proj2"},
							},
						},
					},
				},
			},
			want: []string{"wsa1", "wsa1wsa2", "wsa2"},
		},
		{
			name: "Multiple WSA with intersection, seven scopes",
			args: args{
				wsaList: []v1beta1.WorkloadServiceAccount{
					{
						ObjectMeta: metav1.ObjectMeta{Name: "wsa1"},
						Spec: v1beta1.WorkloadServiceAccountSpec{
							Scope: v1beta1.WorkloadServiceAccountScope{
								Environments: []string{"env1"},
							},
						},
					},
					{
						ObjectMeta: metav1.ObjectMeta{Name: "wsa2"},
						Spec: v1beta1.WorkloadServiceAccountSpec{
							Scope: v1beta1.WorkloadServiceAccountScope{
								Projects: []string{"proj2"},
							},
						},
					},
					{
						ObjectMeta: metav1.ObjectMeta{Name: "wsa3"},
						Spec: v1beta1.WorkloadServiceAccountSpec{
							Scope: v1beta1.WorkloadServiceAccountScope{
								Spaces: []string{"space1"},
							},
						},
					},
				},
			},
			want: []string{"wsa1", "wsa1wsa2", "wsa1wsa2wsa3", "wsa1wsa3", "wsa2", "wsa2wsa3", "wsa3"},
		},
		{
			name: "Multiple WSA with some intersection, three scopes",
			args: args{
				wsaList: []v1beta1.WorkloadServiceAccount{
					{
						ObjectMeta: metav1.ObjectMeta{Name: "wsa1"},
						Spec: v1beta1.WorkloadServiceAccountSpec{
							Scope: v1beta1.WorkloadServiceAccountScope{
								Environments: []string{"env1"},
							},
						},
					},
					{
						ObjectMeta: metav1.ObjectMeta{Name: "wsa2"},
						Spec: v1beta1.WorkloadServiceAccountSpec{
							Scope: v1beta1.WorkloadServiceAccountScope{
								Projects: []string{"proj2"},
							},
						},
					},
					{
						ObjectMeta: metav1.ObjectMeta{Name: "wsa3"},
						Spec: v1beta1.WorkloadServiceAccountSpec{
							Scope: v1beta1.WorkloadServiceAccountScope{
								Environments: []string{"env2"},
							},
						},
					},
				},
			},
			want: []string{"wsa1", "wsa1wsa2", "wsa2", "wsa2wsa3", "wsa3"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equalf(t, tt.want, getScopes(tt.args.wsaList), "getScopes(%v)", tt.args.wsaList)
		})
	}
}
