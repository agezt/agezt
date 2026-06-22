import { describe, expect, it } from "vitest";
import { ACPAgents } from "@/views/ACPAgents";
import { Analyst } from "@/views/Analyst";
import { Cache } from "@/views/Cache";
import { Catalog } from "@/views/Catalog";
import { Chains } from "@/views/Chains";
import { Chat } from "@/views/Chat";
import { Config } from "@/views/Config";
import { Council } from "@/views/Council";
import { FlowStudio } from "@/views/FlowStudio";
import { IncidentPage } from "@/views/IncidentPage";
import { Mission } from "@/views/Mission";
import { Reflect } from "@/views/Reflect";
import { Replay } from "@/views/Replay";
import { Toolbox } from "@/views/Toolbox";

const views = [
  ACPAgents,
  Analyst,
  Cache,
  Catalog,
  Chains,
  Chat,
  Config,
  Council,
  FlowStudio,
  IncidentPage,
  Mission,
  Reflect,
  Replay,
  Toolbox,
];

describe("previously untested view modules", () => {
  it("export renderable component functions", () => {
    for (const View of views) {
      expect(typeof View).toBe("function");
    }
  });
});
