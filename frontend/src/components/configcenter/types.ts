// types.ts — configcenter type definitions (extracted from ConfigCenter.tsx). Day 23 god file split.
//
// Exported types/interfaces + the enum-like type aliases they reference
// (FieldType, ApplyMode) live here so the interface bodies don't depend on
// page.tsx internals. Internal-only types (Section, ReloadBoundary,
// ConfigSchemaResponse, SetResult, AgentConfigEntry, Category) stay in
// page.tsx. Public surface (page.tsx re-exports) unchanged.
//
// The field shape is the daemon's own schema row, narrowed to the fields the
// page reads — the browser names the fields, the daemon names what they are.
import type {
  ConfigSchemaResponse ,
  ConfigSchemaResponseSectionsItemFieldsItem ,
  ConfigValuesResponseFieldsItem,
} from "@/app/api/types.generated";

export type FieldType = "text" | "password" | "number" | "bool" | "csv" | "select";
export type ApplyMode = "live" | "restart";

export type Field = {
  env: ConfigSchemaResponseSectionsItemFieldsItem["env"];
  label: ConfigSchemaResponseSectionsItemFieldsItem["label"];
  type: ConfigSchemaResponseSectionsItemFieldsItem["type"];
  secret: ConfigSchemaResponseSectionsItemFieldsItem["secret"];
  required: ConfigSchemaResponseSectionsItemFieldsItem["required"];
  help?: ConfigSchemaResponseSectionsItemFieldsItem["help"];
  apply: ConfigSchemaResponseSectionsItemFieldsItem["apply"];
  options?: ConfigSchemaResponseSectionsItemFieldsItem["options"];
  read_only?: ConfigSchemaResponseSectionsItemFieldsItem["read_only"];
  locked?: ConfigSchemaResponseSectionsItemFieldsItem["locked"];
};
export type ValueEntry = {
  env: ConfigValuesResponseFieldsItem["env"];
  secret: ConfigValuesResponseFieldsItem["secret"];
  env_pinned: ConfigValuesResponseFieldsItem["env_pinned"];
  set: ConfigValuesResponseFieldsItem["set"];
  value?: ConfigValuesResponseFieldsItem["value"];};
