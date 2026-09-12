package main

import coreapp "github.com/Wen5555/LinkSend/internal/app"

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
