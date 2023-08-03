/*
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

package scheduling

import (
	"fmt"
	"net"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// HostPortUsage tracks HostPort usage within a node. On a node, each <hostIP, hostPort, protocol> used by pods bound
// to the node must be unique. We need to track this to keep an accurate concept of what pods can potentially schedule
// together.
type HostPortUsage struct {
	reserved map[types.NamespacedName][]HostPort
}

type HostPort struct {
	ip       net.IP
	port     int32
	protocol v1.Protocol
}

func (e HostPort) String() string {
	return fmt.Sprintf("IP=%s Port=%d Proto=%s", e.ip, e.port, e.protocol)
}

func (e HostPort) matches(rhs HostPort) bool {
	if e.protocol != rhs.protocol {
		return false
	}
	if e.port != rhs.port {
		return false
	}
	// If IPs are unequal, they don't match unless one is an unspecified address "0.0.0.0" or the IPv6 address "::".
	if !e.ip.Equal(rhs.ip) && !e.ip.IsUnspecified() && !rhs.ip.IsUnspecified() {
		return false
	}
	return true
}

func NewHostPortUsage() *HostPortUsage {
	return &HostPortUsage{
		reserved: map[types.NamespacedName][]HostPort{},
	}
}

// Add adds a port to the HostPortUsage
func (u *HostPortUsage) Add(pod *v1.Pod, hostPorts []HostPort) {
	u.reserved[client.ObjectKeyFromObject(pod)] = hostPorts
}

func (u *HostPortUsage) Get(pod *v1.Pod) ([]HostPort, error) {
	newUsage := getHostPorts(pod)
	for _, newEntry := range newUsage {
		for podKey, entries := range u.reserved {
			for _, existing := range entries {
				if newEntry.matches(existing) && podKey != client.ObjectKeyFromObject(pod) {
					return nil, fmt.Errorf("%s conflicts with existing HostPort configuration %s", newEntry, existing)
				}
			}
		}
	}
	return newUsage, nil
}

// DeletePod deletes all host port usage from the HostPortUsage that were created by the pod with the given name.
func (u *HostPortUsage) DeletePod(key types.NamespacedName) {
	delete(u.reserved, key)
}

func (u *HostPortUsage) DeepCopy() *HostPortUsage {
	if u == nil {
		return nil
	}
	out := &HostPortUsage{}
	u.DeepCopyInto(out)
	return out
}

func (u *HostPortUsage) DeepCopyInto(out *HostPortUsage) {
	out.reserved = map[types.NamespacedName][]HostPort{}
	for k, v := range u.reserved {
		for _, e := range v {
			out.reserved[k] = append(out.reserved[k], HostPort{
				ip:       e.ip,
				port:     e.port,
				protocol: e.protocol,
			})
		}
	}
}

func getHostPorts(pod *v1.Pod) []HostPort {
	var usage []HostPort
	for _, c := range pod.Spec.Containers {
		for _, p := range c.Ports {
			if p.HostPort == 0 {
				continue
			}
			// Per the K8s docs, "If you don't specify the hostIP and protocol explicitly, Kubernetes will use 0.0.0.0
			// as the default hostIP and TCP as the default protocol." In testing, and looking at the code the protocol
			// is defaulted to TCP, but it leaves the IP empty.
			hostIP := p.HostIP
			if hostIP == "" {
				hostIP = "0.0.0.0"
			}
			usage = append(usage, HostPort{
				ip:       net.ParseIP(hostIP),
				port:     p.HostPort,
				protocol: p.Protocol,
			})
		}
	}
	return usage
}
