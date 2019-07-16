package condition

import (
	"github.com/codeready-toolchain/api/pkg/apis/toolchain/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type ScenarioReconciler = func(request reconcile.Request) (reconcile.Result, error)

type Scenario interface {
	Execute() error
}

type ScenarioConditionValue int

const (
	FalseOrUnknown ScenarioConditionValue = iota
	True
)

type ScenarioCondition struct {
	conditionType v1alpha1.ConditionType
	query ScenarioConditionValue
}

type ScenarioSelector interface {
	Add(scenario Scenario) ScenarioSelector
	Select(conditions []v1alpha1.Condition) (Scenario, error)
}

type scenario struct {
	reconciler ScenarioReconciler
	conditions []ScenarioCondition
}

type scenarioSelector struct {
	scenario []scenario
}

type scenarioParameter struct {
	ParameterType v1alpha1.ConditionType
}

func NewScenario(reconcileFunction ScenarioReconciler, conditions...ScenarioCondition) Scenario {
	return &scenario{
		reconciler: reconcileFunction,
		conditions: conditions,
	}
}

func (s *scenario) Execute() error {
	return nil
}

func NewScenarioSelector(scenarios...Scenario) ScenarioSelector {
	return &scenarioSelector{}
}

func (s *scenarioSelector) Add(scenario Scenario) ScenarioSelector {

	return ScenarioSelector(s)
}

func (s *scenarioSelector) Select(conditions []v1alpha1.Condition) (Scenario, error) {
	return nil, nil
}
