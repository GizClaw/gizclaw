package memory

import (
	"context"
	"fmt"
	"reflect"
	"strings"
)

// BindApp returns a borrowed Store view that fixes Scope.AppID while
// preserving caller-supplied UserID, AgentID, and RunID. The view never closes
// or otherwise owns store.
func BindApp(store Store, appID string) (Store, error) {
	if nilInterface(store) {
		return nil, fmt.Errorf("%w: app-scoped store is required", ErrInvalidInput)
	}
	appID = strings.TrimSpace(appID)
	if appID == "" {
		return nil, fmt.Errorf("%w: app-scoped store app id is required", ErrInvalidInput)
	}

	bound := &appStore{store: store, appID: appID}
	waiter, hasWaiter := store.(OperationWaiter)
	processor, hasProcessor := store.(AsyncOperationProcessor)
	statistics, hasStatistics := store.(StatisticsProvider)
	bound.waiter = waiter
	bound.processor = processor
	bound.statistics = statistics

	switch {
	case hasWaiter && hasProcessor && hasStatistics:
		return &appStoreAll{appStore: bound}, nil
	case hasWaiter && hasProcessor:
		return &appStoreWaiterProcessor{appStore: bound}, nil
	case hasWaiter && hasStatistics:
		return &appStoreWaiterStatistics{appStore: bound}, nil
	case hasProcessor && hasStatistics:
		return &appStoreProcessorStatistics{appStore: bound}, nil
	case hasWaiter:
		return &appStoreWaiter{appStore: bound}, nil
	case hasProcessor:
		return &appStoreProcessor{appStore: bound}, nil
	case hasStatistics:
		return &appStoreStatistics{appStore: bound}, nil
	default:
		return bound, nil
	}
}

type appStore struct {
	store      Store
	appID      string
	waiter     OperationWaiter
	processor  AsyncOperationProcessor
	statistics StatisticsProvider
}

func (s *appStore) underlyingMemoryStore() Store { return s.store }

func (s *appStore) Observe(ctx context.Context, observation Observation) (ObserveResult, error) {
	scope, err := s.bindScope(observation.Scope)
	if err != nil {
		return ObserveResult{}, err
	}
	observation = cloneObservation(observation)
	observation.Scope = scope
	result, err := s.store.Observe(ctx, observation)
	return cloneObserveResult(result), err
}

func (s *appStore) Recall(ctx context.Context, query Query) (RecallResult, error) {
	scope, err := s.bindScope(query.Scope)
	if err != nil {
		return RecallResult{}, err
	}
	query = cloneQuery(query)
	query.Scope = scope
	result, err := s.store.Recall(ctx, query)
	return cloneRecallResult(result), err
}

func (s *appStore) Update(ctx context.Context, request UpdateRequest) (Fact, error) {
	scope, err := s.bindScope(request.Scope)
	if err != nil {
		return Fact{}, err
	}
	request = cloneUpdateRequest(request)
	request.Scope = scope
	result, err := s.store.Update(ctx, request)
	return cloneFact(result), err
}

func (s *appStore) Delete(ctx context.Context, request DeleteRequest) error {
	scope, err := s.bindScope(request.Scope)
	if err != nil {
		return err
	}
	request.Scope = scope
	return s.store.Delete(ctx, request)
}

func (s *appStore) bindScope(scope Scope) (Scope, error) {
	switch appID := strings.TrimSpace(scope.AppID); {
	case appID == "":
		scope.AppID = s.appID
	case appID == s.appID:
		scope.AppID = s.appID
	default:
		return Scope{}, fmt.Errorf("%w: scope app id %q conflicts with bound app id", ErrInvalidInput, scope.AppID)
	}
	return scope, nil
}

func (s *appStore) wait(ctx context.Context, request OperationRequest) (ObserveResult, error) {
	scope, err := s.bindScope(request.Scope)
	if err != nil {
		return ObserveResult{}, err
	}
	request.Scope = scope
	result, err := s.waiter.Wait(ctx, request)
	return cloneObserveResult(result), err
}

func (s *appStore) processAsync(ctx context.Context, request OperationRequest) (ObserveResult, error) {
	scope, err := s.bindScope(request.Scope)
	if err != nil {
		return ObserveResult{}, err
	}
	request.Scope = scope
	result, err := s.processor.ProcessAsync(ctx, request)
	return cloneObserveResult(result), err
}

func (s *appStore) stats(ctx context.Context, scope Scope) (Statistics, error) {
	scope, err := s.bindScope(scope)
	if err != nil {
		return Statistics{}, err
	}
	return s.statistics.Stats(ctx, scope)
}

type appStoreWaiter struct{ *appStore }

func (s *appStoreWaiter) Wait(ctx context.Context, request OperationRequest) (ObserveResult, error) {
	return s.wait(ctx, request)
}

type appStoreProcessor struct{ *appStore }

func (s *appStoreProcessor) ProcessAsync(ctx context.Context, request OperationRequest) (ObserveResult, error) {
	return s.processAsync(ctx, request)
}

type appStoreStatistics struct{ *appStore }

func (s *appStoreStatistics) Stats(ctx context.Context, scope Scope) (Statistics, error) {
	return s.stats(ctx, scope)
}

type appStoreWaiterProcessor struct{ *appStore }

func (s *appStoreWaiterProcessor) Wait(ctx context.Context, request OperationRequest) (ObserveResult, error) {
	return s.wait(ctx, request)
}

func (s *appStoreWaiterProcessor) ProcessAsync(ctx context.Context, request OperationRequest) (ObserveResult, error) {
	return s.processAsync(ctx, request)
}

type appStoreWaiterStatistics struct{ *appStore }

func (s *appStoreWaiterStatistics) Wait(ctx context.Context, request OperationRequest) (ObserveResult, error) {
	return s.wait(ctx, request)
}

func (s *appStoreWaiterStatistics) Stats(ctx context.Context, scope Scope) (Statistics, error) {
	return s.stats(ctx, scope)
}

type appStoreProcessorStatistics struct{ *appStore }

func (s *appStoreProcessorStatistics) ProcessAsync(ctx context.Context, request OperationRequest) (ObserveResult, error) {
	return s.processAsync(ctx, request)
}

func (s *appStoreProcessorStatistics) Stats(ctx context.Context, scope Scope) (Statistics, error) {
	return s.stats(ctx, scope)
}

type appStoreAll struct{ *appStore }

func (s *appStoreAll) Wait(ctx context.Context, request OperationRequest) (ObserveResult, error) {
	return s.wait(ctx, request)
}

func (s *appStoreAll) ProcessAsync(ctx context.Context, request OperationRequest) (ObserveResult, error) {
	return s.processAsync(ctx, request)
}

func (s *appStoreAll) Stats(ctx context.Context, scope Scope) (Statistics, error) {
	return s.stats(ctx, scope)
}

func nilInterface(value any) bool {
	if value == nil {
		return true
	}
	reflection := reflect.ValueOf(value)
	switch reflection.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return reflection.IsNil()
	default:
		return false
	}
}

func cloneObservation(observation Observation) Observation {
	observation.Context = cloneMap(observation.Context)
	if observation.Turns != nil {
		observation.Turns = append([]Turn(nil), observation.Turns...)
		for i := range observation.Turns {
			observation.Turns[i].Attributes = cloneMap(observation.Turns[i].Attributes)
		}
	}
	if observation.Facts != nil {
		observation.Facts = append([]FactCandidate(nil), observation.Facts...)
		for i := range observation.Facts {
			observation.Facts[i].Attributes = cloneMap(observation.Facts[i].Attributes)
		}
	}
	return observation
}

func cloneQuery(query Query) Query {
	if query.Filters != nil {
		query.Filters = append([]Filter(nil), query.Filters...)
		for i := range query.Filters {
			query.Filters[i].Value = cloneValue(query.Filters[i].Value)
		}
	}
	return query
}

func cloneUpdateRequest(request UpdateRequest) UpdateRequest {
	if request.Text != nil {
		text := *request.Text
		request.Text = &text
	}
	request.Attributes.Set = cloneMap(request.Attributes.Set)
	request.Attributes.Delete = append([]string(nil), request.Attributes.Delete...)
	return request
}

func cloneObserveResult(result ObserveResult) ObserveResult {
	if result.Facts != nil {
		result.Facts = append([]Fact(nil), result.Facts...)
		for i := range result.Facts {
			result.Facts[i] = cloneFact(result.Facts[i])
		}
	}
	if result.Operation != nil {
		operation := *result.Operation
		result.Operation = &operation
	}
	return result
}

func cloneRecallResult(result RecallResult) RecallResult {
	if result.Matches != nil {
		result.Matches = append([]Match(nil), result.Matches...)
		for i := range result.Matches {
			result.Matches[i].Fact = cloneFact(result.Matches[i].Fact)
		}
	}
	return result
}
