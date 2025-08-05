package models

import (
	"time"
	"gorm.io/gorm"
)

// AppDeployment represents deployment information for a Citizen app
type AppDeployment struct {
	ID          uint      `json:"id" gorm:"primaryKey"`
	AppName     string    `json:"app_name" gorm:"not null;uniqueIndex:idx_app_deployment"`
	Domain      string    `json:"domain"`
	Port        int       `json:"port"`
	Builder     string    `json:"builder"`
	Buildpack   string    `json:"buildpack"`
	GitURL      string    `json:"git_url"`
	GitBranch   string    `json:"git_branch"`
	GitCommit       string    `json:"git_commit"`
	DeploymentLogs  string    `json:"deployment_logs" gorm:"type:text"`
	PortSource      string    `json:"port_source"` // "project.toml", "package.json", "manual", etc.
	Status          string    `json:"status"`     // "deployed", "failed", "pending"
	LastDeploy  time.Time `json:"last_deploy"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	DeletedAt   gorm.DeletedAt `json:"deleted_at" gorm:"index"`
}

// TableName returns the table name for AppDeployment model
func (AppDeployment) TableName() string {
	return "app_deployments"
}

// BeforeCreate runs before creating a new AppDeployment
func (ad *AppDeployment) BeforeCreate(tx *gorm.DB) error {
	if ad.Status == "" {
		ad.Status = "pending"
	}
	return nil
}

// AppDeploymentRequest represents the request payload for creating/updating app deployment
type AppDeploymentRequest struct {
	AppName     string `json:"app_name" binding:"required"`
	Domain      string `json:"domain"`
	Port        int    `json:"port"`
	Builder     string `json:"builder"`
	Buildpack   string `json:"buildpack"`
	GitURL      string `json:"git_url"`
	GitBranch   string `json:"git_branch"`
	PortSource  string `json:"port_source"`
}

// AppDeploymentResponse represents the response payload for app deployment
type AppDeploymentResponse struct {
	ID          uint      `json:"id"`
	AppName     string    `json:"app_name"`
	Domain      string    `json:"domain"`
	Port        int       `json:"port"`
	Builder     string    `json:"builder"`
	Buildpack   string    `json:"buildpack"`
	GitURL      string    `json:"git_url"`
	GitBranch   string    `json:"git_branch"`
	GitCommit   string    `json:"git_commit"`
	PortSource  string    `json:"port_source"`
	Status      string    `json:"status"`
	LastDeploy  time.Time `json:"last_deploy"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
} 