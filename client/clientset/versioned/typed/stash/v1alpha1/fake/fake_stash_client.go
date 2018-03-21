/*
Copyright 2018 The Stash Authors.

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

package fake

import (
	v1alpha1 "github.com/appscode/stash/client/clientset/versioned/typed/stash/v1alpha1"
	rest "k8s.io/client-go/rest"
	testing "k8s.io/client-go/testing"
)

type FakeStashV1alpha1 struct {
	*testing.Fake
}

func (c *FakeStashV1alpha1) Recoveries(namespace string) v1alpha1.RecoveryInterface {
	return &FakeRecoveries{c, namespace}
}

func (c *FakeStashV1alpha1) Repositories(namespace string) v1alpha1.RepositoryInterface {
	return &FakeRepositories{c, namespace}
}

func (c *FakeStashV1alpha1) Restics(namespace string) v1alpha1.ResticInterface {
	return &FakeRestics{c, namespace}
}

// RESTClient returns a RESTClient that is used to communicate
// with API server by this client implementation.
func (c *FakeStashV1alpha1) RESTClient() rest.Interface {
	var ret *rest.RESTClient
	return ret
}