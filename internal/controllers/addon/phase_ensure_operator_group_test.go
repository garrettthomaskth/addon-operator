package addon

import (
	"context"
	"testing"

	operatorsv1 "github.com/operator-framework/api/pkg/operators/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"

	addonsv1alpha1 "github.com/openshift/addon-operator/apis/addons/v1alpha1"
	"github.com/openshift/addon-operator/internal/controllers"
	"github.com/openshift/addon-operator/internal/testutil"
)

func TestEnsureOperatorGroup(t *testing.T) {
	t.Run("ensures OperatorGroup", func(t *testing.T) {
		addonOwnNamespace := &addonsv1alpha1.Addon{
			ObjectMeta: metav1.ObjectMeta{
				Name: "addon-1",
			},
			Spec: addonsv1alpha1.AddonSpec{
				Install: addonsv1alpha1.AddonInstallSpec{
					Type: addonsv1alpha1.OLMOwnNamespace,
					OLMOwnNamespace: &addonsv1alpha1.AddonInstallOLMOwnNamespace{
						AddonInstallOLMCommon: addonsv1alpha1.AddonInstallOLMCommon{
							CatalogSourceImage: "quay.io/osd-addons/test:sha256:04864220677b2ed6244f2e0d421166df908986700647595ffdb6fd9ca4e5098a",
							Namespace:          "addon-system",
						},
					},
				},
			},
		}

		addonAllNamespaces := &addonsv1alpha1.Addon{
			ObjectMeta: metav1.ObjectMeta{
				Name: "addon-1",
			},
			Spec: addonsv1alpha1.AddonSpec{
				Install: addonsv1alpha1.AddonInstallSpec{
					Type: addonsv1alpha1.OLMAllNamespaces,
					OLMAllNamespaces: &addonsv1alpha1.AddonInstallOLMAllNamespaces{
						AddonInstallOLMCommon: addonsv1alpha1.AddonInstallOLMCommon{
							CatalogSourceImage: "quay.io/osd-addons/test:sha256:04864220677b2ed6244f2e0d421166df908986700647595ffdb6fd9ca4e5098a",
							Namespace:          "addon-system",
						},
					},
				},
			},
		}

		tests := []struct {
			name                     string
			addon                    *addonsv1alpha1.Addon
			targetNamespace          string
			expectedTargetNamespaces []string
		}{
			{
				name:                     "OwnNamespace",
				addon:                    addonOwnNamespace,
				targetNamespace:          addonOwnNamespace.Spec.Install.OLMOwnNamespace.Namespace,
				expectedTargetNamespaces: []string{addonOwnNamespace.Spec.Install.OLMOwnNamespace.Namespace},
			},
			{
				name:            "AllNamespaces",
				addon:           addonAllNamespaces,
				targetNamespace: addonAllNamespaces.Spec.Install.OLMAllNamespaces.Namespace,
			},
		}

		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				log := testutil.NewLogger(t)
				c := testutil.NewClient()
				r := &olmReconciler{
					client: c,
					scheme: testutil.NewTestSchemeWithAddonsv1alpha1(),
				}
				addon := test.addon

				// Mock Setup
				c.
					On(
						"Get",
						mock.Anything,
						client.ObjectKey{
							Name:      controllers.DefaultOperatorGroupName,
							Namespace: test.targetNamespace,
						},
						mock.IsType(&operatorsv1.OperatorGroup{}),
					).
					Return(errors.NewNotFound(schema.GroupResource{}, ""))
				var createdOpeatorGroup *operatorsv1.OperatorGroup
				c.
					On(
						"Create",
						mock.Anything,
						mock.IsType(&operatorsv1.OperatorGroup{}),
						mock.Anything,
					).
					Run(func(args mock.Arguments) {
						createdOpeatorGroup = args.Get(1).(*operatorsv1.OperatorGroup)
					}).
					Return(nil)

				// Test
				ctx := controllers.ContextWithLogger(context.Background(), log)
				requeueResult, err := r.ensureOperatorGroup(ctx, addon)
				require.NoError(t, err)
				assert.Equal(t, resultNil, requeueResult)

				if c.AssertCalled(
					t, "Create",
					mock.Anything,
					mock.IsType(&operatorsv1.OperatorGroup{}),
					mock.Anything,
				) {
					assert.Equal(t, controllers.DefaultOperatorGroupName, createdOpeatorGroup.Name)
					assert.Equal(t, test.targetNamespace, createdOpeatorGroup.Namespace)

					assert.Equal(t, test.expectedTargetNamespaces, createdOpeatorGroup.Spec.TargetNamespaces)
				}
			})
		}
	})

	t.Run("guards against invalid configuration", func(t *testing.T) {
		tests := []struct {
			name  string
			addon *addonsv1alpha1.Addon
		}{
			{
				name: "ownNamespace is nil",
				addon: &addonsv1alpha1.Addon{
					ObjectMeta: metav1.ObjectMeta{
						Name: "addon-1",
					},
					Spec: addonsv1alpha1.AddonSpec{
						Install: addonsv1alpha1.AddonInstallSpec{
							Type: addonsv1alpha1.OLMOwnNamespace,
						},
					},
				},
			},
			{
				name: "ownNamespace.namespace is empty",
				addon: &addonsv1alpha1.Addon{
					ObjectMeta: metav1.ObjectMeta{
						Name: "addon-1",
					},
					Spec: addonsv1alpha1.AddonSpec{
						Install: addonsv1alpha1.AddonInstallSpec{
							Type:            addonsv1alpha1.OLMOwnNamespace,
							OLMOwnNamespace: &addonsv1alpha1.AddonInstallOLMOwnNamespace{},
						},
					},
				},
			},
			{
				name: "allNamespaces is nil",
				addon: &addonsv1alpha1.Addon{
					ObjectMeta: metav1.ObjectMeta{
						Name: "addon-1",
					},
					Spec: addonsv1alpha1.AddonSpec{
						Install: addonsv1alpha1.AddonInstallSpec{
							Type: addonsv1alpha1.OLMAllNamespaces,
						},
					},
				},
			},
			{
				name: "allNamespaces.namespace is empty",
				addon: &addonsv1alpha1.Addon{
					ObjectMeta: metav1.ObjectMeta{
						Name: "addon-1",
					},
					Spec: addonsv1alpha1.AddonSpec{
						Install: addonsv1alpha1.AddonInstallSpec{
							Type:             addonsv1alpha1.OLMAllNamespaces,
							OLMAllNamespaces: &addonsv1alpha1.AddonInstallOLMAllNamespaces{},
						},
					},
				},
			},
		}

		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				log := testutil.NewLogger(t)
				c := testutil.NewClient()
				r := &olmReconciler{
					client: c,
					scheme: testutil.NewTestSchemeWithAddonsv1alpha1(),
				}

				// Test
				ctx := controllers.ContextWithLogger(context.Background(), log)
				requeueResult, err := r.ensureOperatorGroup(ctx, test.addon)
				require.NoError(t, err)
				assert.Equal(t, resultStop, requeueResult)

				availableCond := meta.FindStatusCondition(test.addon.Status.Conditions, addonsv1alpha1.Available)
				if assert.NotNil(t, availableCond) {
					assert.Equal(t, metav1.ConditionFalse, availableCond.Status)
					assert.Equal(t, addonsv1alpha1.AddonReasonConfigError, availableCond.Reason)
				}
			})
		}
	})

	t.Run("unsupported install type", func(t *testing.T) {
		addonUnsupported := &addonsv1alpha1.Addon{
			ObjectMeta: metav1.ObjectMeta{
				Name: "addon-1",
			},
			Spec: addonsv1alpha1.AddonSpec{
				Install: addonsv1alpha1.AddonInstallSpec{
					Type: addonsv1alpha1.AddonInstallType("something something"),
				},
			},
		}

		log := testutil.NewLogger(t)
		c := testutil.NewClient()
		r := &olmReconciler{
			client: c,
			scheme: testutil.NewTestSchemeWithAddonsv1alpha1(),
		}

		// Test
		ctx := controllers.ContextWithLogger(context.Background(), log)
		requeueResult, err := r.ensureOperatorGroup(ctx, addonUnsupported.DeepCopy())
		require.NoError(t, err)
		assert.Equal(t, resultStop, requeueResult)

		// indirect sanity check
		// nothing was called on the client and the method signals to stop
	})
}

func TestReconcileOperatorGroup_Adoption(t *testing.T) {
	for name, tc := range map[string]struct {
		OwnedByAddon bool
	}{
		"Already owned by addon": {
			OwnedByAddon: false,
		},
		"Not already owned by addon": {
			OwnedByAddon: true,
		},
	} {
		t.Run(name, func(t *testing.T) {
			operatorGroup := testutil.NewTestOperatorGroup()
			c := testutil.NewClient()

			c.On("Get",
				testutil.IsContext,
				testutil.IsObjectKey,
				testutil.IsOperatorsV1OperatorGroupPtr,
			).Run(func(args mock.Arguments) {
				var og *operatorsv1.OperatorGroup

				if tc.OwnedByAddon {
					og = testutil.NewTestOperatorGroup()
					// Unrelated spec change to force reconciliation
					og.Spec.StaticProvidedAPIs = true
				} else {
					og = testutil.NewTestOperatorGroupWithoutOwner()
				}

				og.DeepCopyInto(args.Get(2).(*operatorsv1.OperatorGroup))
			}).Return(nil)

			c.On("Update",
				testutil.IsContext,
				testutil.IsOperatorsV1OperatorGroupPtr,
				mock.Anything,
			).Return(nil)

			rec := &olmReconciler{
				client: c,
				scheme: testutil.NewTestSchemeWithAddonsv1alpha1(),
			}

			ctx := context.Background()
			err := rec.reconcileOperatorGroup(ctx, operatorGroup.DeepCopy())

			assert.NoError(t, err)
			c.AssertExpectations(t)
		})
	}
}
