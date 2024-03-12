package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/nixmade/orchestrator/httpclient"
	"github.com/rs/zerolog"

	"github.com/nixmade/orchestrator/core"
)

type testApp struct {
	endpoint    string
	bearerToken string
	namespace   string
	entity      string
	logger      zerolog.Logger
	*httpclient.OrchestratorAPI
}

func (t *testApp) getClientState(group string) ([]*core.ClientState, error) {
	url := t.Status(t.namespace, t.entity)

	if group != "" {
		url = t.GroupStatus(t.namespace, t.entity, group)
	}

	var clientTargets []*core.ClientState
	err := httpclient.GetJSON(url, t.bearerToken, &clientTargets)
	if err != nil {
		return nil, err
	}

	return clientTargets, nil
}

func (t *testApp) setTargetVersion(targetVersion string) error {
	return httpclient.PostJSON(t.TargetVersion(t.namespace, t.entity), t.bearerToken, &core.EntityTargetVersion{Version: targetVersion}, nil)
}

func (t *testApp) setRolloutOptions(options *core.RolloutOptions) error {
	return httpclient.PostJSON(t.RolloutOptions(t.namespace, t.entity), t.bearerToken, options, nil)
}

func (t *testApp) postClientTargets(clientTargets []*core.ClientState) error {
	return httpclient.PostJSON(t.Status(t.namespace, t.entity), t.bearerToken, clientTargets, nil)
}

func (t *testApp) rollout(version, group string, clientTargets []*core.ClientState) ([]*core.ClientState, error) {
	if err := t.setTargetVersion(version); err != nil {
		return nil, err
	}

	if err := t.postClientTargets(clientTargets); err != nil {
		t.logger.Error().Err(err).Msg("Failed to post client targets")
		return nil, err
	}

	return t.getClientState(group)
}

func main() {
	logger := zerolog.New(os.Stderr).With().Caller().Timestamp().Logger().Output(zerolog.ConsoleWriter{Out: os.Stderr}).Level(zerolog.DebugLevel)

	hostAddr := flag.String("host", "127.0.0.1", "endpoint address")
	port := flag.String("port", "8080", "endpoint port")

	namespace := flag.String("namespace", "namespace", "namespace")
	entity := flag.String("entity", "entity", "entity name")
	version := flag.String("version", "v1", "version")
	numGroups := flag.Int("groups", 1, "number of groups")
	numTargets := flag.Int("targets", 10, "number of targets")
	successTimeout := flag.Int("successTimeout", 10, "success timeout in secs")
	durationTimeout := flag.Int("durationTimeout", 30, "duration timeout in secs")
	batchPercent := flag.Int("batchPercent", 5, "batch percent")
	successPercent := flag.Int("successPercent", 95, "success percent")
	isError := flag.Bool("error", false, "check true if version needs to be reported error")

	flag.Parse()

	options := core.DefaultRolloutOptions()
	options.BatchPercent = *batchPercent
	options.SuccessPercent = *successPercent
	options.SuccessTimeoutSecs = *successTimeout
	options.DurationTimeoutSecs = *durationTimeout

	endpoint := fmt.Sprintf("http://%s:%s", *hostAddr, *port)

	logger.Info().
		Str("Endpoint", endpoint).
		Str("Namespace", *namespace).
		Str("Entity", *entity).
		Str("Version", *version).
		Int("NumGroups", *numGroups).
		EmbedObject(options).Send()

	// This should be an input to this tool, for now create it
	// jwtToken := os.Getenv("ORCHESTRATOR_API_KEY")

	// if jwtToken == "" {
	// 	log.Error("failed to use jwt token since environment variable is not set ORCHESTRATOR_API_KEY")
	// 	return
	// }

	// log.Infof("Using JWT %s", jwtToken)

	testapp := testApp{
		endpoint:        endpoint,
		bearerToken:     "", //fmt.Sprintf("Bearer %s", jwtToken),
		namespace:       *namespace,
		entity:          *entity,
		logger:          logger,
		OrchestratorAPI: httpclient.NewOrchestratorAPI(endpoint),
	}

	if err := testapp.setRolloutOptions(options); err != nil {
		logger.Error().Err(err).Msg("Failed to report rollout options")
		return
	}

	clientGroupTargets := make(map[string][]*core.ClientState)
	for i := 0; i < *numGroups; i++ {
		group := fmt.Sprintf("Group%d", i)
		clientTargets, err := testapp.getClientState(group)
		if err != nil {
			logger.Error().Err(err).Msg("Failed to get client state")
			return
		}
		if len(clientTargets) <= 0 {
			logger.Info().Msg("No Targets found, creating new targets")
			for i := 0; i < *numTargets; i++ {
				clientTarget := &core.ClientState{
					Name:    fmt.Sprintf("clientTarget%d", i),
					Group:   group,
					Version: "",
					Message: "running successfully",
					IsError: false,
				}
				clientTargets = append(clientTargets, clientTarget)
			}
		} else {
			logger.Info().Str("Group", group).Int("Targets", len(clientTargets)).Msg("Found targets using target state")
		}
		clientTargets, err = testapp.rollout(*version, group, clientTargets)
		if err != nil {
			logger.Error().Err(err).Msg("Failed to rollout testApp")
			return
		}
		clientGroupTargets[group] = clientTargets
	}

	time.Sleep(5 * time.Second)

	for {
		for group, clientGroupTarget := range clientGroupTargets {
			logger.Info().Str("Group", group).Str("Version", *version).Bool("IsError", *isError).Msg("Rolling out")
			for _, clientTarget := range clientGroupTarget {
				clientTarget.Message = "running successfully"
				clientTarget.IsError = false
				if *isError && clientTarget.Version == *version {
					clientTarget.Message = "simulating error"
					clientTarget.IsError = *isError
				}
			}
			clientTargets, err := testapp.rollout(*version, group, clientGroupTarget)
			if err != nil {
				logger.Error().Err(err).Msg("Failed to rollout testApp")
				return
			}
			clientGroupTargets[group] = clientTargets
		}
		time.Sleep(3 * time.Second)
	}
}
