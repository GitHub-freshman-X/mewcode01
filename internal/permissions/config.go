package permissions

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"

	"go.yaml.in/yaml/v4"
)

type FilePaths struct {
	User    string
	Project string
	Local   string
}

type RuleSet struct {
	Session []Rule
	Local   []Rule
	Project []Rule
	User    []Rule
}

type RuleStore struct {
	mu      sync.RWMutex
	session []Rule
	local   []Rule
	project []Rule
	user    []Rule
}

func DefaultFilePaths(workspace string) (FilePaths, error) {
	if workspace == "" {
		return FilePaths{}, errors.New("workspace is required")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return FilePaths{}, fmt.Errorf("resolve home directory: %w", err)
	}
	return FilePaths{
		User:    filepath.Join(home, ".mewcode", "permissions.yaml"),
		Project: filepath.Join(workspace, ".mewcode", "permissions.yaml"),
		Local:   filepath.Join(workspace, ".mewcode", "permissions.local.yaml"),
	}, nil
}

func LoadRuleSet(paths FilePaths) (RuleSet, error) {
	user, err := loadRulesFile(paths.User, ScopeUser)
	if err != nil {
		return RuleSet{}, err
	}
	project, err := loadRulesFile(paths.Project, ScopeProject)
	if err != nil {
		return RuleSet{}, err
	}
	local, err := loadRulesFile(paths.Local, ScopeLocal)
	if err != nil {
		return RuleSet{}, err
	}
	return RuleSet{Local: local, Project: project, User: user}, nil
}

func AppendLocalAllow(paths FilePaths, rule Rule) error {
	if paths.Local == "" {
		return errors.New("local permissions path is required")
	}
	if rule.Effect != EffectAllow {
		return errors.New("only allow rules can be appended")
	}
	if err := os.MkdirAll(filepath.Dir(paths.Local), 0o755); err != nil {
		return fmt.Errorf("create local permissions directory: %w", err)
	}
	existing, err := loadRuleMap(paths.Local)
	if err != nil {
		return err
	}
	existing[rule.Key] = EffectAllow
	return writeRuleMap(paths.Local, existing)
}

func NewRuleStore(rules RuleSet) *RuleStore {
	return &RuleStore{
		session: append([]Rule(nil), rules.Session...),
		local:   append([]Rule(nil), rules.Local...),
		project: append([]Rule(nil), rules.Project...),
		user:    append([]Rule(nil), rules.User...),
	}
}

func (s *RuleStore) AddSessionRule(rule Rule) {
	if s == nil {
		return
	}
	rule.Scope = ScopeSession
	s.mu.Lock()
	defer s.mu.Unlock()
	s.session = append([]Rule{rule}, s.session...)
}

func (s *RuleStore) Find(req Request) (Rule, bool, error) {
	if s == nil {
		return Rule{}, false, nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, layer := range [][]Rule{s.session, s.local, s.project, s.user} {
		for _, rule := range layer {
			ok, err := MatchRule(rule, req)
			if err != nil {
				return Rule{}, false, err
			}
			if ok {
				return rule, true, nil
			}
		}
	}
	return Rule{}, false, nil
}

func loadRulesFile(path string, scope Scope) ([]Rule, error) {
	if path == "" {
		return nil, nil
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read permissions file %s: %w", path, err)
	}
	var root yaml.Node
	if err := yaml.Unmarshal(data, &root); err != nil {
		return nil, fmt.Errorf("parse permissions file %s: %w", path, err)
	}
	return parseRulesNode(path, scope, &root)
}

func loadRuleMap(path string) (map[string]Effect, error) {
	out := make(map[string]Effect)
	if path == "" {
		return out, nil
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return out, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read permissions file %s: %w", path, err)
	}
	var root yaml.Node
	if err := yaml.Unmarshal(data, &root); err != nil {
		return nil, fmt.Errorf("parse permissions file %s: %w", path, err)
	}
	doc := documentContent(&root)
	if doc == nil {
		return out, nil
	}
	rulesNode, err := rulesMappingNode(path, doc)
	if err != nil {
		return nil, err
	}
	if rulesNode == nil {
		return out, nil
	}
	for i := 0; i < len(rulesNode.Content); i += 2 {
		key := rulesNode.Content[i].Value
		effect := Effect(rulesNode.Content[i+1].Value)
		if _, err := ParseRule(key, effect, ScopeLocal, i/2); err != nil {
			return nil, fmt.Errorf("%s: invalid rule %q: %w", path, key, err)
		}
		out[key] = effect
	}
	return out, nil
}

func parseRulesNode(path string, scope Scope, root *yaml.Node) ([]Rule, error) {
	doc := documentContent(root)
	if doc == nil {
		return nil, nil
	}
	rulesNode, err := rulesMappingNode(path, doc)
	if err != nil {
		return nil, err
	}
	if rulesNode == nil {
		return nil, nil
	}
	rules := make([]Rule, 0, len(rulesNode.Content)/2)
	for i := 0; i < len(rulesNode.Content); i += 2 {
		key := rulesNode.Content[i].Value
		effect := Effect(rulesNode.Content[i+1].Value)
		rule, err := ParseRule(key, effect, scope, i/2)
		if err != nil {
			return nil, fmt.Errorf("%s: invalid rule %q: %w", path, key, err)
		}
		rules = append(rules, rule)
	}
	return rules, nil
}

func documentContent(root *yaml.Node) *yaml.Node {
	if root == nil || len(root.Content) == 0 {
		return nil
	}
	return root.Content[0]
}

func rulesMappingNode(path string, doc *yaml.Node) (*yaml.Node, error) {
	if doc.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("%s: permissions file must be a mapping", path)
	}
	var rulesNode *yaml.Node
	for i := 0; i < len(doc.Content); i += 2 {
		key := doc.Content[i].Value
		value := doc.Content[i+1]
		if key != "rules" {
			return nil, fmt.Errorf("%s: unknown field %q", path, key)
		}
		rulesNode = value
	}
	if rulesNode == nil {
		return nil, nil
	}
	if rulesNode.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("%s: rules must be a mapping", path)
	}
	for i := 1; i < len(rulesNode.Content); i += 2 {
		if rulesNode.Content[i].Kind != yaml.ScalarNode {
			return nil, fmt.Errorf("%s: rule effect must be scalar", path)
		}
	}
	return rulesNode, nil
}

func writeRuleMap(path string, rules map[string]Effect) error {
	keys := make([]string, 0, len(rules))
	for key := range rules {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	root := &yaml.Node{Kind: yaml.MappingNode}
	rulesNode := &yaml.Node{Kind: yaml.MappingNode}
	root.Content = append(root.Content, scalarNode("rules"), rulesNode)
	for _, key := range keys {
		rulesNode.Content = append(rulesNode.Content, scalarNode(key), scalarNode(string(rules[key])))
	}
	data, err := yaml.Marshal(root)
	if err != nil {
		return fmt.Errorf("encode local permissions file: %w", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write local permissions file %s: %w", path, err)
	}
	return nil
}

func scalarNode(value string) *yaml.Node {
	return &yaml.Node{Kind: yaml.ScalarNode, Value: value}
}
