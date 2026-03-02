package service

import "github.com/fyltr/angee/api"

// History returns recent git commits from ANGEE_ROOT.
func (p *Platform) History(n int) ([]api.CommitInfo, error) {
	if n <= 0 {
		n = 20
	}

	commits, err := p.Git.Log(n)
	if err != nil {
		return []api.CommitInfo{}, nil
	}

	result := make([]api.CommitInfo, 0, len(commits))
	for _, c := range commits {
		result = append(result, api.CommitInfo{
			SHA:     c.SHA,
			Message: c.Message,
			Author:  c.Author,
			Date:    c.Date,
		})
	}
	return result, nil
}
