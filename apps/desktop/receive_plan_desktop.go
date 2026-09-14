package main

import (
	coreapp "github.com/Wen5555/LinkSend/internal/app"
	"github.com/Wen5555/LinkSend/internal/transfer"
)

func (a *App) IncomingFiles(id string, request coreapp.IncomingFilesRequest) (coreapp.IncomingFilesPage, error) {
	if err := a.workspaceAvailable(); err != nil {
		return coreapp.IncomingFilesPage{}, err
	}
	return a.core.IncomingFiles(id, request)
}

func (a *App) IncomingPlan(id string, request coreapp.IncomingPlanRequest) (coreapp.IncomingPlanPreview, error) {
	if err := a.workspaceAvailable(); err != nil {
		return coreapp.IncomingPlanPreview{}, err
	}
	return a.core.IncomingPlan(id, request)
}

func (a *App) AcceptReceivePlan(id string, revision uint64, digest string) error {
	if err := a.workspaceAvailable(); err != nil {
		return err
	}
	return a.core.AcceptReceivePlan(id, revision, digest)
}

func (a *App) AcceptIncomingDefault(id, attemptID string, revision uint64, remember bool) (coreapp.AcceptIncomingDefaultResult, error) {
	if err := a.workspaceAvailable(); err != nil {
		return coreapp.AcceptIncomingDefaultResult{}, err
	}
	peerID := ""
	profiles := []coreapp.DeviceProfile(nil)
	if task, ok := a.core.Task(id); ok {
		peerID = task.PeerID
		if saved, profileErr := a.core.DeviceProfiles(); profileErr == nil {
			profiles = saved
		}
	}
	policy := receiveConflictPolicy(a.Preferences().ConflictPolicy, peerID, profiles)
	return a.core.AcceptIncomingWithPolicy(id, attemptID, revision, remember, policy)
}

func receiveConflictPolicy(global, peerID string, profiles []coreapp.DeviceProfile) transfer.ConflictPolicy {
	policy := transfer.ConflictPolicy(global)
	if policy != transfer.ConflictKeepBoth && policy != transfer.ConflictSkip && policy != transfer.ConflictError {
		policy = transfer.ConflictKeepBoth
	}
	for _, profile := range profiles {
		configured := transfer.ConflictPolicy(profile.ConflictPolicy)
		if profile.PeerID == peerID && (configured == transfer.ConflictKeepBoth || configured == transfer.ConflictSkip || configured == transfer.ConflictError) {
			return configured
		}
	}
	return policy
}
