package ports

import (
	"fmt"
	"strconv"
	"strings"
	"sync"

	"github.com/fyltr/angee/internal/manifest"
)

type Pool struct {
	Name  string
	Start int
	End   int

	mu     sync.Mutex
	leases map[int]string
}

func ParsePool(name, spec string) (*Pool, error) {
	startText, endText, ok := strings.Cut(spec, "-")
	if !ok {
		return nil, fmt.Errorf("port pool %q range must be start-end", name)
	}
	start, err := strconv.Atoi(strings.TrimSpace(startText))
	if err != nil {
		return nil, fmt.Errorf("port pool %q start: %w", name, err)
	}
	end, err := strconv.Atoi(strings.TrimSpace(endText))
	if err != nil {
		return nil, fmt.Errorf("port pool %q end: %w", name, err)
	}
	if start < 1 || end < start || end > 65535 {
		return nil, fmt.Errorf("port pool %q has invalid range %d-%d", name, start, end)
	}
	return &Pool{Name: name, Start: start, End: end, leases: map[int]string{}}, nil
}

func FromManifest(specs map[string]manifest.PortPool, leases map[string][]manifest.PortLease) (map[string]*Pool, error) {
	pools := make(map[string]*Pool, len(specs))
	for name, spec := range specs {
		pool, err := ParsePool(name, spec.Range)
		if err != nil {
			return nil, err
		}
		for _, lease := range leases[name] {
			if lease.Port < pool.Start || lease.Port > pool.End {
				return nil, fmt.Errorf("lease for pool %q uses out-of-range port %d", name, lease.Port)
			}
			pool.leases[lease.Port] = lease.Owner
		}
		pools[name] = pool
	}
	return pools, nil
}

func (p *Pool) Allocate(owner string) (int, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for port, existingOwner := range p.leases {
		if existingOwner == owner {
			return port, nil
		}
	}
	for port := p.Start; port <= p.End; port++ {
		if _, ok := p.leases[port]; ok {
			continue
		}
		p.leases[port] = owner
		return port, nil
	}
	return 0, fmt.Errorf("port pool %q is exhausted", p.Name)
}

func (p *Pool) Release(owner string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for port, existingOwner := range p.leases {
		if existingOwner == owner {
			delete(p.leases, port)
		}
	}
}

func (p *Pool) Leases() []manifest.PortLease {
	p.mu.Lock()
	defer p.mu.Unlock()
	leases := make([]manifest.PortLease, 0, len(p.leases))
	for port, owner := range p.leases {
		leases = append(leases, manifest.PortLease{Port: port, Owner: owner})
	}
	return leases
}
