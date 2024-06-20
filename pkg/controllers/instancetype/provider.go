/*
Copyright The Kubernetes Authors.

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

package instancetype

import (
	"context"
	"fmt"
	"sync"

	"github.com/awslabs/operatorpkg/option"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"sigs.k8s.io/karpenter/pkg/apis/v1beta1"
	"sigs.k8s.io/karpenter/pkg/cloudprovider"
)

func NewProvider(cloudProvider cloudprovider.CloudProvider) *Provider {
	return &Provider{
		cloudProvider: cloudProvider,
	}
}

type Provider struct {
	cloudProvider cloudprovider.CloudProvider
	instanceTypes sync.Map
}

type Options struct {
	InvalidateCache bool
}

func ConsistentRead(options *Options) {
	options.InvalidateCache = true
}

var DefaultOptions = []option.Function[Options]{
	ConsistentRead,
}

func (p *Provider) Get(ctx context.Context, nodePool *v1beta1.NodePool, optionFuncs ...option.Function[Options]) ([]*cloudprovider.InstanceType, error) {
	options := option.Resolve(append(DefaultOptions, optionFuncs...)...)
	if options.InvalidateCache {
		p.instanceTypes.Delete(nodePool.UID)
	}
	if instanceTypes, ok := p.instanceTypes.Load(nodePool.UID); ok {
		return instanceTypes.([]*cloudprovider.InstanceType), nil
	}
	instanceTypes, err := p.get(ctx, nodePool)
	if err != nil {
		return nil, err
	}
	p.instanceTypes.Store(nodePool.UID, instanceTypes)
	log.FromContext(ctx).
		WithValues("count", len(instanceTypes)).
		Info("discovered instance types")
	return instanceTypes, nil
}

func (p *Provider) get(ctx context.Context, nodePool *v1beta1.NodePool) ([]*cloudprovider.InstanceType, error) {
	instanceTypes, err := p.cloudProvider.GetInstanceTypes(ctx, nodePool)
	if err != nil {
		return nil, fmt.Errorf("getting instance types, %w", err)
	}
	return instanceTypes, nil
}

func (p *Provider) Clear() {
	p.instanceTypes.Range(func(key, value any) bool {
		p.instanceTypes.Delete(key)
		return true
	})
}
