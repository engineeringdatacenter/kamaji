// Copyright 2022 Clastix Labs
// SPDX-License-Identifier: Apache-2.0

package resources

import (
	"context"
	"fmt"

	clientset "k8s.io/client-go/kubernetes"
	bootstraptokenv1 "k8s.io/kubernetes/cmd/kubeadm/app/apis/bootstraptoken/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	kamajiv1alpha1 "github.com/clastix/kamaji/api/v1alpha1"
	"github.com/clastix/kamaji/internal/kubeadm"
	"github.com/clastix/kamaji/internal/resources/utils"
)

type kubeadmPhase int

const (
	PhaseUploadConfigKubeadm kubeadmPhase = iota
	PhaseUploadConfigKubelet
	PhaseBootstrapToken
)

func (d kubeadmPhase) String() string {
	return [...]string{"PhaseUploadConfigKubeadm", "PhaseUploadConfigKubelet", "PhaseAddonCoreDNS", "PhaseAddonKubeProxy", "PhaseBootstrapToken"}[d]
}

type KubeadmPhase struct {
	Client   client.Client
	Name     string
	Phase    kubeadmPhase
	checksum string
}

func (r *KubeadmPhase) isStatusEqual(tenantControlPlane *kamajiv1alpha1.TenantControlPlane) bool {
	i, err := r.GetStatus(tenantControlPlane)
	if err != nil {
		return true
	}

	status, ok := i.(*kamajiv1alpha1.KubeadmPhaseStatus)
	if !ok {
		return false
	}

	return status.Checksum == r.checksum
}

func (r *KubeadmPhase) SetKubeadmConfigChecksum(checksum string) {
	r.checksum = checksum
}

func (r *KubeadmPhase) ShouldStatusBeUpdated(_ context.Context, tenantControlPlane *kamajiv1alpha1.TenantControlPlane) bool {
	return !r.isStatusEqual(tenantControlPlane)
}

func (r *KubeadmPhase) ShouldCleanup(*kamajiv1alpha1.TenantControlPlane) bool {
	return false
}

func (r *KubeadmPhase) CleanUp(context.Context, *kamajiv1alpha1.TenantControlPlane) (bool, error) {
	return false, nil
}

func (r *KubeadmPhase) Define(context.Context, *kamajiv1alpha1.TenantControlPlane) error {
	return nil
}

func (r *KubeadmPhase) GetKubeadmFunction() (func(clientset.Interface, *kubeadm.Configuration) ([]byte, error), error) {
	switch r.Phase {
	case PhaseUploadConfigKubeadm:
		return kubeadm.UploadKubeadmConfig, nil
	case PhaseUploadConfigKubelet:
		return kubeadm.UploadKubeletConfig, nil
	case PhaseBootstrapToken:
		return func(client clientset.Interface, config *kubeadm.Configuration) ([]byte, error) {
			bootstrapTokensEnrichment(config.InitConfiguration.BootstrapTokens)

			return nil, kubeadm.BootstrapToken(client, config)
		}, nil
	default:
		return nil, fmt.Errorf("no available functionality for phase %s", r.Phase)
	}
}

func bootstrapTokensEnrichment(bootstrapTokens []bootstraptokenv1.BootstrapToken) {
	var bootstrapToken bootstraptokenv1.BootstrapToken
	if len(bootstrapTokens) > 0 {
		bootstrapToken = bootstrapTokens[0]
	}

	enrichBootstrapToken(&bootstrapToken)
	bootstrapTokens[0] = bootstrapToken
}

func enrichBootstrapToken(bootstrapToken *bootstraptokenv1.BootstrapToken) {
	if bootstrapToken.Token == nil {
		bootstrapToken.Token = &bootstraptokenv1.BootstrapTokenString{}
	}

	if bootstrapToken.Token.ID == "" {
		bootstrapToken.Token.ID = fmt.Sprintf("%s.%s", utils.RandomString(6), utils.RandomString(16))
	}
}

func (r *KubeadmPhase) GetClient() client.Client {
	return r.Client
}

func (r *KubeadmPhase) GetTmpDirectory() string {
	return ""
}

func (r *KubeadmPhase) GetName() string {
	return r.Name
}

func (r *KubeadmPhase) UpdateTenantControlPlaneStatus(ctx context.Context, tenantControlPlane *kamajiv1alpha1.TenantControlPlane) error {
	logger := log.FromContext(ctx, "resource", r.GetName(), "phase", r.Phase.String())

	status, err := r.GetStatus(tenantControlPlane)
	if err != nil {
		logger.Error(err, "unable to update the status")

		return err
	}

	status.SetChecksum(r.checksum)

	return nil
}

func (r *KubeadmPhase) GetStatus(tenantControlPlane *kamajiv1alpha1.TenantControlPlane) (kamajiv1alpha1.KubeadmConfigChecksumDependant, error) {
	switch r.Phase {
	case PhaseUploadConfigKubeadm:
		return &tenantControlPlane.Status.KubeadmPhase.UploadConfigKubeadm, nil
	case PhaseUploadConfigKubelet:
		return &tenantControlPlane.Status.KubeadmPhase.UploadConfigKubelet, nil
	case PhaseBootstrapToken:
		return &tenantControlPlane.Status.KubeadmPhase.BootstrapToken, nil
	default:
		return nil, fmt.Errorf("%s is not a right kubeadm phase", r.Phase)
	}
}

func (r *KubeadmPhase) CreateOrUpdate(ctx context.Context, tenantControlPlane *kamajiv1alpha1.TenantControlPlane) (controllerutil.OperationResult, error) {
	logger := log.FromContext(ctx, "resource", r.GetName(), "phase", r.Phase.String())

	return KubeadmPhaseCreate(ctx, r, logger, tenantControlPlane)
}
