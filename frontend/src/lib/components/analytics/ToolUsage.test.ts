// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from "vitest";
import { mount, tick, unmount } from "svelte";
// @ts-ignore
import ToolUsage from "./ToolUsage.svelte";
import { AnalyticsService, type DbSignalSessionsResponse } from "../../api/generated/index.js";
import { router } from "../../stores/router.svelte.js";
import { analytics } from "../../stores/analytics.svelte.js";

const missingRates = {
  analyzed_calls: 0,
  known_outcome_calls: 0,
  empty_calls: 0,
  repeated_calls: 0,
  recovered_sequences: 0,
  abandoned_sequences: 0,
  open_sequences: 0,
  unknown_sequences: 0,
  missing_calls: 0,
  empty_rate: null,
  repeat_rate: null,
  recovery_rate: null,
};

describe("ToolUsage", () => {
  afterEach(() => {
    analytics.tools = null;
    // @ts-ignore
    analytics.errors = {
      ...analytics.errors,
      tools: null,
    };
    document.body.innerHTML = "";
    vi.restoreAllMocks();
  });

  it("renders ranked per-tool analysis rows", async () => {
    analytics.tools = {
      total_calls: 6,
      by_category: [
        { category: "Read", count: 3, pct: 50 },
        { category: "Bash", count: 2, pct: 33.3 },
      ],
      by_agent: [],
      by_tool: [
        {
          ...missingRates,
          tool_name: "Read",
          category: "Read",
          call_count: 3,
          session_count: 2,
          pct: 50,
        },
        {
          ...missingRates,
          tool_name: "Bash",
          category: "Bash",
          call_count: 2,
          session_count: 1,
          pct: 33.3,
        },
      ],
      trend: [
        {
          date: "2024-06-03",
          by_category: { Read: 3, Bash: 2 },
        },
        {
          date: "2024-06-10",
          by_category: { Read: 1 },
        },
      ],
    };

    const component = mount(ToolUsage, { target: document.body });
    await tick();

    expect(document.body.textContent).toContain("Tool Usage");
    expect(document.body.textContent).toContain("6 calls");
    expect(document.body.textContent).toContain("Top tools");
    expect(document.body.textContent).toContain("Read");
    expect(document.body.textContent).toContain("3");
    expect(document.body.textContent).toContain("2 sessions");
    expect(document.body.textContent).toContain("50%");
    expect(document.body.textContent).toContain("Bash");
    expect(document.body.textContent).toContain("1 session");
    expect(document.body.textContent).not.toContain("1 sessions");
    expect(document.body.textContent).toContain("33.3%");
    expect(document.body.textContent).toContain("By Category");
    expect(document.body.textContent).toContain("Weekly Trend");

    unmount(component);
  });

  it("renders loading state while the first fetch is in flight", async () => {
    analytics.tools = null;
    analytics.loading = { ...analytics.loading, tools: true };

    const component = mount(ToolUsage, { target: document.body });
    await tick();

    expect(document.body.textContent).toContain("Loading tool usage...");
    expect(document.body.textContent).not.toContain("No tool usage data");

    analytics.loading = { ...analytics.loading, tools: false };
    unmount(component);
  });

  it("renders empty state without tool rows", async () => {
    analytics.tools = {
      total_calls: 0,
      by_category: [],
      by_agent: [],
      by_tool: [],
      trend: [],
    };

    const component = mount(ToolUsage, { target: document.body });
    await tick();

    expect(document.body.textContent).toContain("No tool usage data");

    unmount(component);
  });
  function seedRates() {
    analytics.loading = { ...analytics.loading, tools: false };
    analytics.tools = {
      total_calls: 7,
      by_category: [{ category: "Grep", count: 7, pct: 100 }],
      by_agent: [],
      trend: [],
      by_tool: [
        {
          ...missingRates,
          tool_name: "Grep",
          category: "Grep",
          call_count: 7,
          session_count: 4,
          pct: 100,
          analyzed_calls: 6,
          known_outcome_calls: 5,
          empty_calls: 5,
          repeated_calls: 2,
          recovered_sequences: 1,
          abandoned_sequences: 1,
          open_sequences: 1,
          unknown_sequences: 1,
          missing_calls: 1,
          empty_rate: 1,
          repeat_rate: 1 / 3,
          recovery_rate: 0.5,
        },
      ],
    };
  }

  it("shows partial coverage and each rate's denominator", async () => {
    seedRates();
    const component = mount(ToolUsage, { target: document.body });
    await tick();
    expect(document.body.textContent).toContain("6/7");
    expect(document.body.textContent).toContain("100.0%");
    expect(document.body.textContent).toContain("33.3%");
    expect(document.body.textContent).toContain("50.0%");
    expect(
      document.querySelector('[title="5/5 empty results among known outcomes."]'),
    ).not.toBeNull();
    expect(
      document.querySelector('[title="1/2 recovered sequences. Open: 1. Unknown: 1."]'),
    ).not.toBeNull();
    unmount(component);
  });

  it("opens contributing sessions with the active filters and ignores an older response", async () => {
    seedRates();
    analytics.project = "project-a";
    analytics.model = "model-b";
    let resolveOlder!: (value: DbSignalSessionsResponse) => void;
    const older = new Promise<DbSignalSessionsResponse>((resolve) => (resolveOlder = resolve));
    const example = {
      session_id: "recovered",
      project: "project-a",
      agent: "claude",
      date: "2025-06-02",
      is_automated: false,
      outcome: "",
      health_score: null,
      health_grade: null,
      signal_total: 1,
      reason_code: "tool_recovery_rate",
      excerpt: "",
      message_ordinal: 2,
      failure_signals: 0,
      retries: 0,
      edit_churn: 0,
    };
    const fetch = vi
      .spyOn(AnalyticsService, "getApiV1AnalyticsSignalSessions")
      .mockReturnValueOnce(older)
      .mockResolvedValue({ signal: "tool_recovery_rate", total: 1, sessions: [example] });
    const navigate = vi.spyOn(router, "navigateToSession").mockImplementation(() => {});
    const component = mount(ToolUsage, { target: document.body });
    await tick();
    (document.querySelector('[aria-label="Grep Empty rate"]') as HTMLButtonElement).click();
    await tick();
    (document.querySelector('[aria-label="Grep Recovery rate"]') as HTMLButtonElement).click();
    await tick();
    await tick();
    expect(fetch).toHaveBeenLastCalledWith(
      expect.objectContaining({
        project: "project-a",
        model: "model-b",
        tool_name: "Grep",
        signal: "tool_recovery_rate",
      }),
      expect.anything(),
    );
    expect(fetch.mock.calls[0]?.[1]?.signal?.aborted).toBe(true);
    resolveOlder({
      signal: "tool_empty_rate",
      total: 1,
      sessions: [{ ...example, session_id: "stale", project: "stale-project" }],
    });
    await tick();
    await tick();
    expect(document.body.textContent).not.toContain("stale-project");
    (document.querySelector("a.evidence-row") as HTMLAnchorElement).click();
    expect(navigate).toHaveBeenCalledWith("recovered", { msg: "2" });
    unmount(component);
    analytics.project = "";
    analytics.model = "";
  });

  it("refreshes evidence after a filter changes", async () => {
    seedRates();
    const fetch = vi
      .spyOn(AnalyticsService, "getApiV1AnalyticsSignalSessions")
      .mockResolvedValue({ signal: "tool_empty_rate", total: 0, sessions: [] });
    const component = mount(ToolUsage, { target: document.body });
    await tick();
    (document.querySelector('[aria-label="Grep Empty rate"]') as HTMLButtonElement).click();
    await tick();
    analytics.project = "project-b";
    await tick();
    expect(fetch).toHaveBeenLastCalledWith(
      expect.objectContaining({ project: "project-b", tool_name: "Grep" }),
      expect.anything(),
    );
    unmount(component);
    analytics.project = "";
  });
});
