package apiserver

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	wpv1 "github.com/benji/wordpress-manager-operator/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// SiteLabel groups pods/resources belonging to one WordPressSite.
const SiteLabel = "wp.benji.dev/site"

// ConditionDTO is a status condition surfaced to the UI.
type ConditionDTO struct {
	Type           string `json:"type"`
	Status         string `json:"status"`
	Reason         string `json:"reason,omitempty"`
	Message        string `json:"message,omitempty"`
	LastTransition string `json:"lastTransition,omitempty"`
}

// PodStatusDTO is the diagnostic status of a single pod.
type PodStatusDTO struct {
	Name     string `json:"name"`
	Phase    string `json:"phase"`
	Ready    string `json:"ready"`             // e.g. "0/1"
	Reason   string `json:"reason,omitempty"`  // ImagePullBackOff, Unschedulable, ...
	Message  string `json:"message,omitempty"` // why it is stuck
	Restarts int32  `json:"restarts"`
	Node     string `json:"node,omitempty"`
	Created  string `json:"created,omitempty"`
}

// EventDTO is a Kubernetes event related to the site.
type EventDTO struct {
	Type     string `json:"type"` // Normal | Warning
	Reason   string `json:"reason"`
	Message  string `json:"message"`
	Count    int32  `json:"count"`
	Object   string `json:"object"` // Kind/Name
	LastSeen string `json:"lastSeen,omitempty"`
}

// SiteStatusDTO is the full diagnostic payload for a site.
type SiteStatusDTO struct {
	Phase      string         `json:"phase"`
	Message    string         `json:"message,omitempty"`
	Conditions []ConditionDTO `json:"conditions,omitempty"`
	Pods       []PodStatusDTO `json:"pods,omitempty"`
	Events     []EventDTO     `json:"events,omitempty"`
}

// latestConditionMessage returns the Ready condition message (or the last one).
func latestConditionMessage(s *wpv1.WordPressSite) string {
	for _, c := range s.Status.Conditions {
		if c.Type == "Ready" {
			return c.Message
		}
	}
	if n := len(s.Status.Conditions); n > 0 {
		return s.Status.Conditions[n-1].Message
	}
	return ""
}

// getSiteStatus returns conditions, pods (with stuck-reason) and related events
// so the UI can explain Error/Pending states.
func (s *Server) getSiteStatus(w http.ResponseWriter, r *http.Request) {
	site, err := s.fetch(r)
	if err != nil {
		notFoundOr500(w, err)
		return
	}
	out := SiteStatusDTO{Phase: site.Status.Phase, Message: latestConditionMessage(site)}
	for _, c := range site.Status.Conditions {
		lt := ""
		if !c.LastTransitionTime.IsZero() {
			lt = c.LastTransitionTime.Format(time.RFC3339)
		}
		out.Conditions = append(out.Conditions, ConditionDTO{
			Type: c.Type, Status: string(c.Status), Reason: c.Reason, Message: c.Message, LastTransition: lt,
		})
	}

	// Pods belonging to this site.
	pods := &corev1.PodList{}
	if err := s.K8s.List(r.Context(), pods,
		client.InNamespace(s.Namespace), client.MatchingLabels{SiteLabel: site.Name}); err == nil {
		for i := range pods.Items {
			out.Pods = append(out.Pods, podStatus(&pods.Items[i]))
		}
	}

	// Events involving the site's objects (deployment "<name>" + pods/RS "<name>-*").
	events := &corev1.EventList{}
	if err := s.K8s.List(r.Context(), events, client.InNamespace(s.Namespace)); err == nil {
		for i := range events.Items {
			e := &events.Items[i]
			n := e.InvolvedObject.Name
			if n == site.Name || strings.HasPrefix(n, site.Name+"-") {
				out.Events = append(out.Events, eventDTO(e))
			}
		}
		sort.Slice(out.Events, func(i, j int) bool { return out.Events[i].LastSeen > out.Events[j].LastSeen })
		if len(out.Events) > 25 {
			out.Events = out.Events[:25]
		}
	}

	writeJSON(w, http.StatusOK, out)
}

func podStatus(p *corev1.Pod) PodStatusDTO {
	ready, total := 0, len(p.Spec.Containers)
	var restarts int32
	var reason, message string
	for _, cs := range p.Status.ContainerStatuses {
		if cs.Ready {
			ready++
		}
		restarts += cs.RestartCount
		if reason == "" && cs.State.Waiting != nil {
			reason, message = cs.State.Waiting.Reason, cs.State.Waiting.Message
		} else if reason == "" && cs.State.Terminated != nil && cs.State.Terminated.Reason != "" {
			reason, message = cs.State.Terminated.Reason, cs.State.Terminated.Message
		}
	}
	// Pending with no container reason → look at the scheduling condition
	// (e.g. "pod has unbound immediate PersistentVolumeClaims").
	if reason == "" && p.Status.Phase == corev1.PodPending {
		for _, c := range p.Status.Conditions {
			if c.Type == corev1.PodScheduled && c.Status == corev1.ConditionFalse {
				reason, message = c.Reason, c.Message
			}
		}
	}
	if reason == "" {
		reason = p.Status.Reason
	}
	created := ""
	if !p.CreationTimestamp.IsZero() {
		created = p.CreationTimestamp.Format(time.RFC3339)
	}
	return PodStatusDTO{
		Name:     p.Name,
		Phase:    string(p.Status.Phase),
		Ready:    fmt.Sprintf("%d/%d", ready, total),
		Reason:   reason,
		Message:  message,
		Restarts: restarts,
		Node:     p.Spec.NodeName,
		Created:  created,
	}
}

func eventDTO(e *corev1.Event) EventDTO {
	last := e.LastTimestamp.Time
	if last.IsZero() {
		last = e.EventTime.Time
	}
	ls := ""
	if !last.IsZero() {
		ls = last.Format(time.RFC3339)
	}
	count := e.Count
	if count == 0 {
		count = 1
	}
	return EventDTO{
		Type:     e.Type,
		Reason:   e.Reason,
		Message:  e.Message,
		Count:    count,
		Object:   e.InvolvedObject.Kind + "/" + e.InvolvedObject.Name,
		LastSeen: ls,
	}
}
