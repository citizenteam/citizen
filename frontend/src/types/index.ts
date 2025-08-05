export interface App {
  name: string;
  status?: string;
}

export interface Domain {
  domain: string;
}

export interface CustomDomain {
  app_name: string;
  domain: string;
  is_active: boolean;
  created_at: string;
  updated_at: string;
}

export interface PublicAppSetting {
  id: number;
  app_name: string;
  is_public: boolean;
  created_at: string;
  updated_at: string;
}

export interface FormData {
  appName: string;
  domain: string;
  customDomain: string;
  port: string;
  gitUrl: string;
  envVars: string;
}

export interface AppInfo {
  domains?: string[];
  running?: boolean;
  deployed?: boolean;
  ports: { http?: string; https?: string };
  port?: string;
  git_url?: string;
  git_branch?: string;
  builder?: string;
  raw: Record<string, string>;
}

export interface MessageType {
  text: string;
  type: 'success' | 'error' | 'info';
}

export interface DockerConnectionResponse {
  id?: number;
  username: string;
  is_active: boolean;
  created_at?: string;
  updated_at?: string;
  connected: boolean;
}

export interface DockerConnectionRequest {
  username: string;
  access_token: string;
} 