// types.ts — channels type definitions (extracted from Channels.tsx). Day 23 god file split.
//
// Only exported type/interface declarations live here. Internal types
// stay in page.tsx because they aren't consumed outside the
// package's barrel. Public surface (page.tsx re-exports) unchanged.

import type { ChannelListResponseChannelsItem } from "@/app/api/types.generated";

export type ChannelField = {
  env: string;
  label: string;
  secret?: boolean;
  required?: boolean;
  help?: string;
  set?: boolean;
  value?: string;
  env_pinned?: boolean;
};
export type MediaCaps = {
  image_in?: boolean;
  voice_in?: boolean;
  image_out?: boolean;
  voice_out?: boolean;
};
export interface ChannelProbe {
  accounts?: number;
  configured_accounts?: number;
  live_accounts?: number;
  roundtrip_status?: string;
  roundtrip_ready?: boolean;
  mode?: string;
  note?: string;
}
export type ChannelAccount = {
  label: string;
  configured?: boolean;
  live?: boolean;
  probe?: ChannelProbe;
  fields?: ChannelField[] | null;
};
export interface ChannelRow {
  kind: ChannelListResponseChannelsItem["kind"];
  display: ChannelListResponseChannelsItem["display"];
  description?: ChannelListResponseChannelsItem["description"];
  transport?: ChannelListResponseChannelsItem["transport"];
  duplex?: ChannelListResponseChannelsItem["duplex"];
  media?: MediaCaps;
  config_section?: ChannelListResponseChannelsItem["config_section"];
  docs_url?: ChannelListResponseChannelsItem["docs_url"];
  connect_method?: ChannelListResponseChannelsItem["connect_method"];
  setup_steps?: ChannelListResponseChannelsItem["setup_steps"];
  configured?: ChannelListResponseChannelsItem["configured"];
  live?: ChannelListResponseChannelsItem["live"];
  probe?: ChannelProbe;
  fields?: ChannelField[] | null;
  accounts?: ChannelAccount[] | null;
}
