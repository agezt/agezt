// types.ts — channels type definitions (extracted from Channels.tsx). Day 23 god file split.
//
// Only exported type/interface declarations live here. Internal types
// stay in page.tsx because they aren't consumed outside the
// package's barrel. Public surface (page.tsx re-exports) unchanged.

export interface ChannelField {
  env: string;
  label: string;
  secret?: boolean;
  required?: boolean;
  help?: string;
  set?: boolean;
  value?: string;
  env_pinned?: boolean;
}
export interface MediaCaps {
  image_in?: boolean;
  voice_in?: boolean;
  image_out?: boolean;
  voice_out?: boolean;
}
export interface ChannelProbe {
  accounts?: number;
  configured_accounts?: number;
  live_accounts?: number;
  roundtrip_status?: string;
  roundtrip_ready?: boolean;
  mode?: string;
  note?: string;
}
export interface ChannelAccount {
  label: string;
  configured?: boolean;
  live?: boolean;
  probe?: ChannelProbe;
  fields: ChannelField[];
}
export interface ChannelRow {
  kind: string;
  display: string;
  description?: string;
  transport?: string;
  duplex?: boolean;
  media?: MediaCaps;
  config_section?: string;
  docs_url?: string;
  connect_method?: string;
  setup_steps?: string[];
  configured?: boolean;
  live?: boolean;
  probe?: ChannelProbe;
  fields: ChannelField[];
  accounts?: ChannelAccount[];
}
