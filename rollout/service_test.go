package rollout

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	"github.com/argoproj/argo-rollouts/pkg/apis/rollouts/v1alpha1"
	"github.com/argoproj/argo-rollouts/utils/conditions"
)

func newService(name string, port int, selector map[string]string) *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: metav1.NamespaceDefault,
		},
		Spec: corev1.ServiceSpec{
			Selector: selector,
			Ports: []corev1.ServicePort{{
				Protocol:   "TCP",
				Port:       int32(port),
				TargetPort: intstr.FromInt(port),
			}},
		},
	}
}

func TestGetPreviewAndActiveServices(t *testing.T) {

	f := newFixture(t)
	defer f.Close()
	expActive := newService("active", 80, nil)
	expPreview := newService("preview", 80, nil)
	f.kubeobjects = append(f.kubeobjects, expActive)
	f.kubeobjects = append(f.kubeobjects, expPreview)
	f.serviceLister = append(f.serviceLister, expActive, expPreview)
	rollout := &v1alpha1.Rollout{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: metav1.NamespaceDefault,
		},
		Spec: v1alpha1.RolloutSpec{
			Strategy: v1alpha1.RolloutStrategy{
				BlueGreen: &v1alpha1.BlueGreenStrategy{
					PreviewService: "preview",
					ActiveService:  "active",
				},
			},
		},
	}
	c, _, _ := f.newController(noResyncPeriodFunc)
	t.Run("Get Both", func(t *testing.T) {
		preview, active, err := c.getPreviewAndActiveServices(rollout)
		assert.Nil(t, err)
		assert.Equal(t, expPreview, preview)
		assert.Equal(t, expActive, active)
	})
	t.Run("Preview not found", func(t *testing.T) {
		noPreviewSvcRollout := rollout.DeepCopy()
		noPreviewSvcRollout.Spec.Strategy.BlueGreen.PreviewService = "not-preview"
		_, _, err := c.getPreviewAndActiveServices(noPreviewSvcRollout)
		assert.NotNil(t, err)
		assert.True(t, errors.IsNotFound(err))
	})
	t.Run("Active not found", func(t *testing.T) {
		noActiveSvcRollout := rollout.DeepCopy()
		noActiveSvcRollout.Spec.Strategy.BlueGreen.ActiveService = "not-active"
		_, _, err := c.getPreviewAndActiveServices(noActiveSvcRollout)
		assert.NotNil(t, err)
		assert.True(t, errors.IsNotFound(err))
	})

	t.Run("Invalid Spec: No Active Svc", func(t *testing.T) {
		noActiveSvcRollout := rollout.DeepCopy()
		noActiveSvcRollout.Spec.Strategy.BlueGreen.ActiveService = ""
		_, _, err := c.getPreviewAndActiveServices(noActiveSvcRollout)
		assert.NotNil(t, err)
		assert.EqualError(t, err, "Invalid Spec: Rollout missing field ActiveService")
	})

}

func TestActiveServiceNotFound(t *testing.T) {
	f := newFixture(t)
	defer f.Close()

	r := newBlueGreenRollout("foo", 1, nil, "active-svc", "preview-svc")
	r.Status.Conditions = []v1alpha1.RolloutCondition{}
	f.rolloutLister = append(f.rolloutLister, r)
	f.objects = append(f.objects, r)
	previewSvc := newService("preview-svc", 80, nil)
	notUsedActiveSvc := newService("active-svc", 80, nil)
	f.kubeobjects = append(f.kubeobjects, previewSvc)
	f.serviceLister = append(f.serviceLister, previewSvc)

	patchIndex := f.expectPatchRolloutAction(r)
	f.runExpectError(getKey(r, t), true)

	patch := f.getPatchedRollout(patchIndex)
	expectedPatch := `{
			"status": {
				"conditions": [%s]
			}
		}`
	_, pausedCondition := newProgressingCondition(conditions.ServiceNotFoundReason, notUsedActiveSvc, "")
	assert.Equal(t, calculatePatch(r, fmt.Sprintf(expectedPatch, pausedCondition)), patch)
}

func TestPreviewServiceNotFound(t *testing.T) {
	f := newFixture(t)
	defer f.Close()

	r := newBlueGreenRollout("foo", 1, nil, "active-svc", "preview-svc")
	r.Status.Conditions = []v1alpha1.RolloutCondition{}
	f.rolloutLister = append(f.rolloutLister, r)
	f.objects = append(f.objects, r)
	activeSvc := newService("active-svc", 80, nil)
	notUsedPreviewSvc := newService("preview-svc", 80, nil)
	f.kubeobjects = append(f.kubeobjects, activeSvc)
	f.serviceLister = append(f.serviceLister)

	patchIndex := f.expectPatchRolloutAction(r)
	f.runExpectError(getKey(r, t), true)

	patch := f.getPatchedRollout(patchIndex)
	expectedPatch := `{
			"status": {
				"conditions": [%s]
			}
		}`
	_, pausedCondition := newProgressingCondition(conditions.ServiceNotFoundReason, notUsedPreviewSvc, "")
	assert.Equal(t, calculatePatch(r, fmt.Sprintf(expectedPatch, pausedCondition)), patch)
}
