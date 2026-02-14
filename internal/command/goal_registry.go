package command

import (
	"fmt"
	"log"
	"sort"
)

// GoalRegistry manages available goals (built-in and discovered)
type GoalRegistry interface {
	// List returns all available goal names
	List() []string

	// Get retrieves a goal by name
	Get(name string) (*Goal, error)

	// GetAllGoals returns all goals as a slice
	GetAllGoals() []Goal

	// Reload refreshes the registry (for future hot-reload support)
	Reload() error
}

// DynamicGoalRegistry implements GoalRegistry with support for built-in and user-defined goals
type DynamicGoalRegistry struct {
	builtInGoals     []Goal
	discoveredGoals  map[string]*Goal
	goalDiscovery    *GoalDiscovery
	mergedGoals      map[string]*Goal
	orderedGoalNames []string
}

// NewDynamicGoalRegistry creates a new dynamic goal registry
func NewDynamicGoalRegistry(builtInGoals []Goal, discovery *GoalDiscovery) *DynamicGoalRegistry {
	registry := &DynamicGoalRegistry{
		builtInGoals:    builtInGoals,
		discoveredGoals: make(map[string]*Goal),
		goalDiscovery:   discovery,
		mergedGoals:     make(map[string]*Goal),
	}

	// Perform initial load
	if err := registry.Reload(); err != nil {
		log.Printf("warning: failed to load discovered goals: %v", err)
	}

	return registry
}

// List returns all available goal names
func (r *DynamicGoalRegistry) List() []string {
	return r.orderedGoalNames
}

// Get retrieves a goal by name
func (r *DynamicGoalRegistry) Get(name string) (*Goal, error) {
	goal, exists := r.mergedGoals[name]
	if !exists {
		return nil, fmt.Errorf("goal not found: %s", name)
	}
	return goal, nil
}

// Reload refreshes the registry by re-discovering goals
func (r *DynamicGoalRegistry) Reload() error {
	// Clear discovered goals
	r.discoveredGoals = make(map[string]*Goal)

	// Discover goal paths
	paths := r.goalDiscovery.DiscoverGoalPaths()

	// Load goals from each discovered path
	// Paths are pre-sorted by priority (highest first), so only keep first occurrence
	for _, path := range paths {
		// Scan for .json goal files.
		candidates, err := FindGoalFiles(path)
		if err != nil {
			log.Printf("warning: failed to scan goal directory %s: %v", path, err)
			continue
		}

		for _, candidate := range candidates {
			goal, err := LoadGoalFromFile(candidate.Path)
			if err != nil {
				log.Printf("warning: failed to load goal from %s: %v", candidate.Path, err)
				continue
			}

			// Only add if not already present (first path wins = highest priority)
			if _, exists := r.discoveredGoals[goal.Name]; !exists {
				r.discoveredGoals[goal.Name] = goal
			}
		}

		// Scan for .prompt.md files (VS Code prompt files).
		promptCandidates, err := FindPromptFiles(path)
		if err != nil {
			log.Printf("warning: failed to scan prompt files in %s: %v", path, err)
			continue
		}
		for _, candidate := range promptCandidates {
			pf, err := LoadPromptFile(candidate.Path)
			if err != nil {
				log.Printf("warning: failed to load prompt file %s: %v", candidate.Path, err)
				continue
			}
			goal := PromptFileToGoal(pf)
			if _, exists := r.discoveredGoals[goal.Name]; !exists {
				r.discoveredGoals[goal.Name] = goal
			}
		}
	}

	// Also scan dedicated prompt file paths (.github/prompts, configured paths).
	promptPaths := r.goalDiscovery.DiscoverPromptFilePaths()
	for _, path := range promptPaths {
		promptCandidates, err := FindPromptFiles(path)
		if err != nil {
			log.Printf("warning: failed to scan prompt files in %s: %v", path, err)
			continue
		}
		for _, candidate := range promptCandidates {
			pf, err := LoadPromptFile(candidate.Path)
			if err != nil {
				log.Printf("warning: failed to load prompt file %s: %v", candidate.Path, err)
				continue
			}
			goal := PromptFileToGoal(pf)
			if _, exists := r.discoveredGoals[goal.Name]; !exists {
				r.discoveredGoals[goal.Name] = goal
			}
		}
	}

	// Merge built-in and discovered goals
	// User goals override built-ins on collision
	r.mergedGoals = make(map[string]*Goal)

	// Add built-in goals first
	for i := range r.builtInGoals {
		goal := &r.builtInGoals[i]
		r.mergedGoals[goal.Name] = goal
	}

	// Override with discovered goals (user goals win)
	for name, goal := range r.discoveredGoals {
		r.mergedGoals[name] = goal
	}

	// Create sorted list of goal names
	r.orderedGoalNames = make([]string, 0, len(r.mergedGoals))
	for name := range r.mergedGoals {
		r.orderedGoalNames = append(r.orderedGoalNames, name)
	}
	sort.Strings(r.orderedGoalNames)

	return nil
}

// GetAllGoals returns all goals as a slice (for listGoals compatibility)
func (r *DynamicGoalRegistry) GetAllGoals() []Goal {
	goals := make([]Goal, 0, len(r.mergedGoals))
	for _, goal := range r.mergedGoals {
		goals = append(goals, *goal) // Creates a shallow copy
	}

	// Sort by name for consistent ordering
	sort.Slice(goals, func(i, j int) bool {
		return goals[i].Name < goals[j].Name
	})

	return goals
}
