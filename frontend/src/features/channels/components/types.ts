// types.ts — channels type definitions (extracted from Channels.tsx). Day 23 god file split.
//
// Only exported type/interface declarations live here. Internal types
// stay in page.tsx because they aren't consumed outside the
// package's barrel. Public surface (page.tsx re-exports) unchanged.

import type {
  ChannelListResponseChannelsItem ,
  ChannelListResponseChannelsItemAccountsItem ,
  ChannelListResponseChannelsItemAccountsItemFieldsItem ,
  ChannelListResponseChannelsItemMedia ,
  ChannelListResponseChannelsItemProbe,
} from "@/app/api/types.generated";

export type ChannelField = {
  env: ChannelListResponseChannelsItemAccountsItemFieldsItem["env"];
  label: ChannelListResponseChannelsItemAccountsItemFieldsItem["label"];
  secret: ChannelListResponseChannelsItemAccountsItemFieldsItem["secret"];
  required: ChannelListResponseChannelsItemAccountsItemFieldsItem["required"];
  help: ChannelListResponseChannelsItemAccountsItemFieldsItem["help"];
  set: ChannelListResponseChannelsItemAccountsItemFieldsItem["set"];
  value?: ChannelListResponseChannelsItemAccountsItemFieldsItem["value"];
  env_pinned: ChannelListResponseChannelsItemAccountsItemFieldsItem["env_pinned"];};
export type MediaCaps = {
  image_in?: ChannelListResponseChannelsItemMedia["image_in"];
  voice_in?: ChannelListResponseChannelsItemMedia["voice_in"];
  image_out?: ChannelListResponseChannelsItemMedia["image_out"];
  voice_out?: ChannelListResponseChannelsItemMedia["voice_out"];};
export interface ChannelProbe {
  accounts?: ChannelListResponseChannelsItemProbe["accounts"];
  configured_accounts?: ChannelListResponseChannelsItemProbe["configured_accounts"];
  live_accounts?: ChannelListResponseChannelsItemProbe["live_accounts"];
  roundtrip_status?: ChannelListResponseChannelsItemProbe["roundtrip_status"];
  roundtrip_ready?: ChannelListResponseChannelsItemProbe["roundtrip_ready"];
  mode?: ChannelListResponseChannelsItemProbe["mode"];
}
export type ChannelAccount = {
  label: ChannelListResponseChannelsItemAccountsItem["label"];
  configured?: ChannelListResponseChannelsItemAccountsItem["configured"];
  live?: ChannelListResponseChannelsItemAccountsItem["live"];
  probe?: ChannelListResponseChannelsItemAccountsItem["probe"];
  fields?: ChannelListResponseChannelsItemAccountsItem["fields"];};
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
