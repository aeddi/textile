package collections

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/textileio/go-threads/api/client"
	s "github.com/textileio/go-threads/store"
)

// Scope specifies the scope of the Config
type Scope int

const (
	// Dev is the scope for development values
	Dev Scope = iota
	// Beta is the scope for beta values
	Beta Scope = iota
	// Prod is the scope for production values
	Prod
)

// ConfigItem represents a single config value
type ConfigItem struct {
	ID         string
	UniqueName string
	Name       string
	Values     map[Scope]string
	ProjectID  string
	Created    int64
	Updated    int64
}

// ProjectConfig provides the project config api
type ProjectConfig struct {
	threads *client.Client
	storeID *uuid.UUID
	token   string
}

func (s Scope) String() string {
	return [...]string{"Dev", "Beta", "Prod"}[s]
}

// All lists all scopes
func (s Scope) All() []Scope {
	return []Scope{
		Dev,
		Beta,
		Prod,
	}
}

// GetName returns the entity name
func (p *ProjectConfig) GetName() string {
	return "Config"
}

// GetInstance returns the ProjectConfig instance
func (p *ProjectConfig) GetInstance() interface{} {
	return &ProjectConfig{}
}

// GetIndexes returns the indexes
func (p *ProjectConfig) GetIndexes() []*s.IndexConfig {
	return []*s.IndexConfig{{
		Path:   "UniqueName",
		Unique: true,
	}, {
		Path: "ProjectID",
	}}
}

// GetStoreID returns the store id
func (p *ProjectConfig) GetStoreID() *uuid.UUID {
	return p.storeID
}

// Create creates a new config
func (p *ProjectConfig) Create(ctx context.Context, name string, values map[Scope]string, projectID string) (*ConfigItem, error) {
	validName, err := toValidName(name)
	if err != nil {
		return nil, err
	}
	ctx = AuthCtx(ctx, p.token)
	config := &ConfigItem{
		UniqueName: fmt.Sprintf("%v-%v", projectID, validName),
		Name:       validName,
		Values:     values,
		ProjectID:  projectID,
		Created:    time.Now().Unix(),
		Updated:    time.Now().Unix(),
	}
	if err := p.threads.ModelCreate(ctx, p.storeID.String(), p.GetName(), config); err != nil {
		return nil, err
	}
	return config, nil
}

// Get fetches a single config
func (p *ProjectConfig) Get(ctx context.Context, projectID string, name string) (*ConfigItem, error) {
	ctx = AuthCtx(ctx, p.token)
	query := s.JSONWhere("ProjectID").Eq(projectID).JSONAnd("Name").Eq(name)
	res, err := p.threads.ModelFind(ctx, p.storeID.String(), p.GetName(), query, []*ConfigItem{})
	if err != nil {
		return nil, err
	}
	configItems := res.([]*ConfigItem)
	if len(configItems) == 0 {
		return nil, nil
	}
	return configItems[0], nil
}

// List lists all ConfigItems for a project
func (p *ProjectConfig) List(ctx context.Context, projectID string) ([]*ConfigItem, error) {
	ctx = AuthCtx(ctx, p.token)
	query := s.JSONWhere("ProjectID").Eq(projectID)
	res, err := p.threads.ModelFind(ctx, p.storeID.String(), p.GetName(), query, []*ConfigItem{})
	if err != nil {
		return nil, err
	}
	return res.([]*ConfigItem), nil
}

// Save saves a config
func (p *ProjectConfig) Save(ctx context.Context, config *ConfigItem) error {
	ctx = AuthCtx(ctx, p.token)
	return p.threads.ModelSave(ctx, p.storeID.String(), p.GetName(), config)
}

// Delete deletes a config
func (p *ProjectConfig) Delete(ctx context.Context, id string) error {
	ctx = AuthCtx(ctx, p.token)
	return p.threads.ModelDelete(ctx, p.storeID.String(), p.GetName(), id)
}