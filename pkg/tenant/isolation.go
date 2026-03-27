package tenant

import (
	"context"
	"fmt"
	"time"

	"github.com/flowpulse/flowpulse/pkg/model"
	"github.com/golang-jwt/jwt/v5"
)

// Manager handles tenant lifecycle and RBAC.
type Manager struct {
	tenants   map[string]*model.Tenant
	users     map[string]*model.User
	jwtSecret []byte
}

func NewManager(jwtSecret []byte) *Manager {
	return &Manager{
		tenants:   make(map[string]*model.Tenant),
		users:     make(map[string]*model.User),
		jwtSecret: jwtSecret,
	}
}

func (m *Manager) CreateTenant(_ context.Context, tenant model.Tenant) error {
	if _, exists := m.tenants[tenant.ID]; exists {
		return fmt.Errorf("tenant %s already exists", tenant.ID)
	}
	tenant.CreatedAt = time.Now()
	tenant.UpdatedAt = time.Now()
	m.tenants[tenant.ID] = &tenant
	return nil
}

func (m *Manager) GetTenant(_ context.Context, id string) (*model.Tenant, error) {
	t, ok := m.tenants[id]
	if !ok {
		return nil, fmt.Errorf("tenant %s not found", id)
	}
	return t, nil
}

func (m *Manager) ListTenants(_ context.Context) []model.Tenant {
	result := make([]model.Tenant, 0, len(m.tenants))
	for _, t := range m.tenants {
		result = append(result, *t)
	}
	return result
}

func (m *Manager) DeleteTenant(_ context.Context, id string) error {
	if _, ok := m.tenants[id]; !ok {
		return fmt.Errorf("tenant %s not found", id)
	}
	delete(m.tenants, id)
	return nil
}

func (m *Manager) CreateUser(_ context.Context, user model.User) error {
	if _, exists := m.users[user.ID]; exists {
		return fmt.Errorf("user %s already exists", user.ID)
	}
	if _, ok := m.tenants[user.TenantID]; !ok {
		return fmt.Errorf("tenant %s not found", user.TenantID)
	}
	m.users[user.ID] = &user
	return nil
}

// IssueToken generates a JWT for a user with tenant and role claims.
func (m *Manager) IssueToken(userID string, expiry time.Duration) (string, error) {
	user, ok := m.users[userID]
	if !ok {
		return "", fmt.Errorf("user %s not found", userID)
	}

	now := time.Now()
	claims := jwt.MapClaims{
		"user_id":   user.ID,
		"tenant_id": user.TenantID,
		"role":      string(user.Role),
		"iat":       now.Unix(),
		"exp":       now.Add(expiry).Unix(),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(m.jwtSecret)
}

// CheckPermission verifies a user has the required role.
func (m *Manager) CheckPermission(claims *model.TokenClaims, requiredRole model.Role) bool {
	if claims == nil {
		return false
	}
	switch requiredRole {
	case model.RoleViewer:
		return true
	case model.RoleOperator:
		return claims.Role == model.RoleOperator || claims.Role == model.RoleAdmin
	case model.RoleAdmin:
		return claims.Role == model.RoleAdmin
	}
	return false
}

// ScopeQuery injects tenant_id filtering for data isolation.
// Admin users can optionally override to view cross-tenant data.
func (m *Manager) ScopeQuery(claims *model.TokenClaims, requestedTenant string) string {
	if claims.Role == model.RoleAdmin && requestedTenant != "" {
		return requestedTenant
	}
	return claims.TenantID
}
