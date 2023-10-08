package config

import (
	"context"

	"github.com/lmxia/syncer/utils"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/rest"
	"k8s.io/klog/v2"

	"github.com/lmxia/syncer/pkg/known"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

func GetHubConfig(kubeClientSet kubernetes.Interface, HubAPIServer, localNamespace string) (*rest.Config, error) {
	secret, err := kubeClientSet.CoreV1().
		Secrets(localNamespace).
		Get(context.TODO(), known.HubSecretName, metav1.GetOptions{})
	if err != nil && !apierrors.IsNotFound(err) {
		klog.Errorf("failed to get secret of parent cluster: %v", err)
		return nil, err
	}
	if err == nil {
		klog.Infof("found existing secretFromParentCluster '%s/%s' that can be used to access parent cluster",
			localNamespace, known.HubSecretName)

		if string(secret.Data[known.ClusterAPIServerURLKey]) != HubAPIServer {
			klog.Warningf("the parent url got changed from %q to %q",
				secret.Data[known.ClusterAPIServerURLKey],
				HubAPIServer)
		} else {
			parentDedicatedKubeConfig, err2 := utils.GenerateKubeConfigFromToken(
				HubAPIServer,
				string(secret.Data[corev1.ServiceAccountTokenKey]),
				secret.Data[corev1.ServiceAccountRootCAKey],
			)
			if err2 == nil {
				return parentDedicatedKubeConfig, nil
			}
			return nil, err2
		}
	}
	return nil, err
}
