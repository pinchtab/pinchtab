// Re-export generated types from Go
export type {
  Profile,
  Instance,
  Agent,
  ActivityEvent,
  Settings,
  ScreencastSettings,
  BrowserSettings,
  ServerInfo,
  CreateProfileRequest,
  LaunchInstanceRequest,
} from '../generated/types'

// View type (frontend only)
export type View = 'profiles' | 'instances' | 'agents' | 'settings'
