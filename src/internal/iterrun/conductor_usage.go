package iterrun

import (
	"context"
	"errors"

	"github.com/x85446/claudecodetricks/src/internal/conductor"
	"github.com/x85446/claudecodetricks/src/internal/conductor/model"
	"github.com/x85446/claudecodetricks/src/internal/conductor/provider"
)

// RefreshConductorUsage binds only identities returned by an authenticated collector.
func RefreshConductorUsage(ctx context.Context, s *conductor.Service, id string, expected int64, bind bool) (model.Usage, error) {
	c, err := s.Config()
	if err != nil {
		return model.Usage{}, err
	}
	if c.Revision != expected {
		return model.Usage{}, conductor.ErrConflict
	}
	index := -1
	for i, sub := range c.Subscriptions {
		if sub.ID == id {
			index = i
			break
		}
	}
	if index < 0 {
		return model.Usage{}, ErrConductorNotFound
	}
	sub := c.Subscriptions[index]
	u, err := provider.CollectUsage(ctx, sub)
	if err != nil {
		return u, err
	}
	if u.AccountID == "" || u.PoolID == "" || !u.Supported {
		return u, errors.New("authenticated account and allowance identity unavailable; authenticate the selected CLI and retry")
	}
	if sub.AccountID != "" && (sub.AccountID != u.AccountID || sub.PoolID != u.PoolID) {
		return u, errors.New("collector account differs from subscription binding; create a separate subscription for that account")
	}
	if sub.AccountID == "" || sub.PoolID == "" {
		if !bind {
			return u, errors.New("subscription is unbound; refresh with bind enabled after reviewing the authenticated account")
		}
		c.Subscriptions[index].AccountID = u.AccountID
		c.Subscriptions[index].PoolID = u.PoolID
		if _, err = s.Apply(expected, c); err != nil {
			return u, err
		}
	} else {
		current, e := s.Config()
		if e != nil {
			return u, e
		}
		if current.Revision != expected {
			return u, conductor.ErrConflict
		}
	}
	return u, s.PutUsage(u)
}
