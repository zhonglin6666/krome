package client

import (
	k8sclient "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	kromeclient "github.com/zhonglin6666/krome/pkg/client/clientset/versioned"
)

// Client defines a client with kubernetes and krome client
type Client struct {
	K8sClient   k8sclient.Interface
	KromeClient kromeclient.Interface
}

func NewClient(mgr manager.Manager) (*Client, error) {
	return newForConfig(mgr.GetConfig())
}

func newForConfig(c *rest.Config) (*Client, error) {
	k8sClient, err := k8sclient.NewForConfig(c)
	if err != nil {
		return nil, err
	}

	kromeClient, err := kromeclient.NewForConfig(c)
	if err != nil {
		return nil, err
	}

	return &Client{
		K8sClient:   k8sClient,
		KromeClient: kromeClient,
	}, nil
}
