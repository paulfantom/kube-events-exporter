/*
Copyright 2020 Red Hat, Inc. All rights reserved.

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

package collector

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"

	v1 "k8s.io/api/core/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/tools/cache"
)

// EventCollector is a prometeus.Collector that bundles all the metrics related
// to Kubernetes Events.
type EventCollector struct {
	eventsTotal *prometheus.CounterVec

	informerFactory informers.SharedInformerFactory
}

// NewEventCollector returns a prometheus.Collector collecting metrics about
// Kubernetes Events.
func NewEventCollector() *EventCollector {
	return &EventCollector{
		eventsTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "kube_events_total",
			Help: "Count of all Kubernetes Events",
		}, []string{"type", "involved_object_namespace", "involved_object_kind", "reason"}),
	}
}

// Describe implements the prometheus.Collector interface.
func (collector *EventCollector) Describe(ch chan<- *prometheus.Desc) {
	collector.eventsTotal.Describe(ch)
}

// Collect implements the prometheus.Collector interface.
func (collector *EventCollector) Collect(ch chan<- prometheus.Metric) {
	collector.eventsTotal.Collect(ch)
}

// WithInformerFactory adds a informers.SharedInformerFactory to the collector.
func (collector *EventCollector) WithInformerFactory(factory informers.SharedInformerFactory) {
	collector.informerFactory = factory
}

// Run starts updating EventCollector metrics.
func (collector *EventCollector) Run(stopCh <-chan struct{}) {
	startRunning := time.Now()
	eventsTotalInformer := collector.informerFactory.Core().V1().Events().Informer()
	eventsTotalInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			ev := obj.(*v1.Event)
			// Count only Events created after the exporter starts running.
			if beforeLastEvent(startRunning, ev) {
				// FIXME: take into account the event count.
				collector.increaseEventsTotal(ev, 1)
			}
		},
		UpdateFunc: func(oldObj, newObj interface{}) {
			oldEv := oldObj.(*v1.Event)
			newEv := newObj.(*v1.Event)
			// Count only Events updated after the exporter starts running.
			if beforeLastEvent(startRunning, newEv) {
				nbNew := updatedEventNb(oldEv, newEv)
				collector.increaseEventsTotal(newEv, float64(nbNew))
			}
		},
	})
	go collector.informerFactory.Start(stopCh)
}

func (collector *EventCollector) increaseEventsTotal(event *v1.Event, nbNew float64) {
	collector.eventsTotal.With(prometheus.Labels{
		"type":                      event.Type,
		"involved_object_namespace": event.InvolvedObject.Namespace,
		"involved_object_kind":      event.InvolvedObject.Kind,
		"reason":                    event.Reason,
	}).Add(nbNew)
}

func beforeLastEvent(t time.Time, ev *v1.Event) bool {
	if ev.Series != nil && !ev.Series.LastObservedTime.IsZero() {
		return t.Before(ev.Series.LastObservedTime.Time)
	}

	return t.Before(ev.LastTimestamp.Time)
}

func updatedEventNb(oldEv, newEv *v1.Event) int32 {
	if newEv.Series != nil {
		if oldEv.Series != nil {
			return newEv.Series.Count - oldEv.Series.Count
		}
		// When event is emitted for the first time it's written to the API
		// server without series field set.
		return newEv.Series.Count
	}

	return newEv.Count - oldEv.Count
}
