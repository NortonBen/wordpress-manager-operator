package apiserver

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	wpv1 "github.com/benji/wordpress-manager-operator/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestPodStatusPendingUnschedulable(t *testing.T) {
	p := &corev1.Pod{
		Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "wordpress"}}},
		Status: corev1.PodStatus{
			Phase: corev1.PodPending,
			Conditions: []corev1.PodCondition{{
				Type: corev1.PodScheduled, Status: corev1.ConditionFalse,
				Reason: "Unschedulable", Message: "pod has unbound immediate PersistentVolumeClaims",
			}},
		},
	}
	st := podStatus(p)
	if st.Phase != "Pending" || st.Ready != "0/1" || st.Reason != "Unschedulable" {
		t.Fatalf("unexpected pod status: %+v", st)
	}
	if !strings.Contains(st.Message, "unbound") {
		t.Errorf("expected PVC message, got %q", st.Message)
	}
}

func TestPodStatusWaitingReason(t *testing.T) {
	p := &corev1.Pod{
		Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "wordpress"}}},
		Status: corev1.PodStatus{
			Phase: corev1.PodPending,
			ContainerStatuses: []corev1.ContainerStatus{{
				Name: "wordpress", RestartCount: 3,
				State: corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{
					Reason: "ImagePullBackOff", Message: "back-off pulling image",
				}},
			}},
		},
	}
	st := podStatus(p)
	if st.Reason != "ImagePullBackOff" || st.Restarts != 3 {
		t.Errorf("unexpected pod status: %+v", st)
	}
}

func TestLatestConditionMessage(t *testing.T) {
	s := &wpv1.WordPressSite{}
	s.Status.Conditions = []metav1.Condition{{
		Type: "Ready", Status: metav1.ConditionFalse, Reason: "Error", Message: "workload: boom",
	}}
	if got := latestConditionMessage(s); got != "workload: boom" {
		t.Errorf("latestConditionMessage = %q", got)
	}
}

func TestSiteStatusEndpoint(t *testing.T) {
	h := newTestServer(t)
	tok := loginToken(t, h)
	if w := req(t, h, "POST", "/api/v1/sites", tok,
		`{"name":"blog-acme","domain":"blog.acme.example"}`); w.Code != http.StatusCreated {
		t.Fatalf("create: %d", w.Code)
	}
	w := req(t, h, "GET", "/api/v1/sites/blog-acme/status", tok, "")
	if w.Code != http.StatusOK {
		t.Fatalf("status: %d", w.Code)
	}
	var out SiteStatusDTO
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	// No real kubelet in the fake cluster → no pods, but the call must succeed.
	if len(out.Pods) != 0 {
		t.Errorf("expected no pods in mock cluster, got %d", len(out.Pods))
	}
}
