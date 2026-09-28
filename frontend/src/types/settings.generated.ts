// CODE GENERATED - DO NOT EDIT MANUALLY
// To change settings, edit internal/config/settings_schema.json and run: go run tools/settings-generator/main.go

export interface SettingsData {
  baidu_app_id: string;
  baidu_secret_key: string;
  close_to_tray: boolean;
  freshrss_api_password: string;
  freshrss_auto_sync_interval: number;
  freshrss_server_url: string;
  freshrss_username: string;
  llm_api_key: string;
  llm_endpoint: string;
  llm_model: string;
  proxy_host: string;
  proxy_mode: string;
  proxy_password: string;
  proxy_port: string;
  proxy_type: string;
  proxy_username: string;
  startup_on_boot: boolean;
  update_check_enabled: boolean;
}

export const settingsDefaults: SettingsData = {
  baidu_app_id: '',
  baidu_secret_key: '',
  close_to_tray: true,
  freshrss_api_password: '',
  freshrss_auto_sync_interval: 30,
  freshrss_server_url: '',
  freshrss_username: '',
  llm_api_key: '',
  llm_endpoint: '',
  llm_model: '',
  proxy_host: '127.0.0.1',
  proxy_mode: 'system',
  proxy_password: '',
  proxy_port: '7890',
  proxy_type: 'http',
  proxy_username: '',
  startup_on_boot: false,
  update_check_enabled: true,
};
