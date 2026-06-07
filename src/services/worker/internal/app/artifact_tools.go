package app

import (
	"log/slog"

	sharedconfig "arkloop/services/shared/config"
	"arkloop/services/shared/objectstore"
	workerdata "arkloop/services/worker/internal/data"
	"arkloop/services/worker/internal/llm"
	"arkloop/services/worker/internal/routing"
	"arkloop/services/worker/internal/tools"
	audiottstool "arkloop/services/worker/internal/tools/builtin/audio_tts"
	documentwritetool "arkloop/services/worker/internal/tools/builtin/document_write"
	frameextracttool "arkloop/services/worker/internal/tools/builtin/frame_extract"
	imagegeneratetool "arkloop/services/worker/internal/tools/builtin/image_generate"
	markdowntopdftool "arkloop/services/worker/internal/tools/builtin/markdown_to_pdf"
	videoconcattool "arkloop/services/worker/internal/tools/builtin/video_concat"
	videogeneratetool "arkloop/services/worker/internal/tools/builtin/video_generate"
)

// registerStoredArtifactTools wires tools that require persisted artifact storage.
func registerStoredArtifactTools(
	toolRegistry *tools.Registry,
	executors map[string]tools.Executor,
	specs []llm.ToolSpec,
	store objectstore.Store,
	sandboxExecutor tools.Executor,
	db workerdata.QueryDB,
	configResolver sharedconfig.Resolver,
	routingLoader *routing.ConfigLoader,
) ([]llm.ToolSpec, bool, error) {
	if toolRegistry == nil || executors == nil || store == nil {
		return specs, false, nil
	}

	artifactExecutor := documentwritetool.NewToolExecutor(store)
	audioTTSExecutor := audiottstool.NewToolExecutor(store, db, configResolver, routingLoader)
	imageExecutor := imagegeneratetool.NewToolExecutor(store, db, configResolver, routingLoader)
	videoExecutor := videogeneratetool.NewToolExecutor(store, db, configResolver, routingLoader)
	videoConcatExecutor := videoconcattool.NewToolExecutor(store)
	frameExtractExecutor := frameextracttool.NewToolExecutor(store)
	markdownToPDFExecutor := markdowntopdftool.NewToolExecutor(store, sandboxExecutor)
	registered := false
	for _, item := range []struct {
		agentSpec tools.AgentToolSpec
		llmSpec   llm.ToolSpec
		executor  tools.Executor
	}{
		{agentSpec: documentwritetool.CreateArtifactAgentSpec, llmSpec: documentwritetool.CreateArtifactLlmSpec, executor: artifactExecutor},
		{agentSpec: documentwritetool.AgentSpec, llmSpec: documentwritetool.LlmSpec, executor: artifactExecutor},
		{agentSpec: markdowntopdftool.AgentSpec, llmSpec: markdowntopdftool.LlmSpec, executor: markdownToPDFExecutor},
		{agentSpec: audiottstool.AgentSpec, llmSpec: audiottstool.LlmSpec, executor: audioTTSExecutor},
		{agentSpec: imagegeneratetool.AgentSpec, llmSpec: imagegeneratetool.LlmSpec, executor: imageExecutor},
		{agentSpec: videogeneratetool.AgentSpec, llmSpec: videogeneratetool.LlmSpec, executor: videoExecutor},
		{agentSpec: videoconcattool.AgentSpec, llmSpec: videoconcattool.LlmSpec, executor: videoConcatExecutor},
		{agentSpec: frameextracttool.AgentSpec, llmSpec: frameextracttool.LlmSpec, executor: frameExtractExecutor},
	} {
		wasRegistered, err := registerToolIfMissing(toolRegistry, item.agentSpec)
		if err != nil {
			return nil, false, err
		}
		registered = registered || wasRegistered
		executors[item.agentSpec.Name] = item.executor
		specs = appendToolSpecIfMissing(specs, item.llmSpec)
	}

	// Diagnostic: log actual registered set instead of the previous hard-coded
	// "[create_artifact document_write markdown_to_pdf image_generate]" string.
	registeredNames := toolRegistry.ListNames()
	slog.Info("artifact_tools: full registered set", "tools", registeredNames)

	return specs, registered, nil
}

func registerToolIfMissing(registry *tools.Registry, spec tools.AgentToolSpec) (bool, error) {
	if registry == nil {
		return false, nil
	}
	if _, ok := registry.Get(spec.Name); ok {
		return false, nil
	}
	if err := registry.Register(spec); err != nil {
		return false, err
	}
	return true, nil
}

func appendToolSpecIfMissing(specs []llm.ToolSpec, spec llm.ToolSpec) []llm.ToolSpec {
	for _, existing := range specs {
		if existing.Name == spec.Name {
			return specs
		}
	}
	return append(specs, spec)
}
