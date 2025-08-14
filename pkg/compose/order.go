package compose

import (
	"sort"

	"gopkg.in/yaml.v3"
)

type composeFile struct {
	Name     string             `yaml:"name"`
	Services map[string]service `yaml:"services"`
}

type service struct {
	DependsOn map[string]any `yaml:"depends_on"`
}

func OrderFromComposeYAML(data []byte) (order []string, names []string) {
	var cf composeFile
	if err := yaml.Unmarshal(data, &cf); err != nil {
		return nil, nil
	}
	for n := range cf.Services {
		names = append(names, n)
	}
	// Build adjacency and indegree
	adj := map[string][]string{}
	indeg := map[string]int{}
	for n := range cf.Services {
		indeg[n] = 0
	}
	for n, svc := range cf.Services {
		for dep := range svc.DependsOn {
			adj[dep] = append(adj[dep], n)
			indeg[n]++
		}
	}
	// Kahn's algorithm
	q := []string{}
	for n, d := range indeg {
		if d == 0 {
			q = append(q, n)
		}
	}
	sort.Strings(q)
	for len(q) > 0 {
		cur := q[0]
		q = q[1:]
		order = append(order, cur)
		for _, nb := range adj[cur] {
			indeg[nb]--
			if indeg[nb] == 0 {
				q = append(q, nb)
				sort.Strings(q)
			}
		}
	}
	if len(order) != len(names) {
		// cycle or missing; fall back to alpha
		order = make([]string, len(names))
		copy(order, names)
		sort.Strings(order)
	}
	return order, names
}

func ParseProjectName(data []byte) string {
	var cf composeFile
	if err := yaml.Unmarshal(data, &cf); err != nil {
		return ""
	}
	return cf.Name
}
