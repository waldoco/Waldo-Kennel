import { describe, expect, it } from "vitest";
import { DEFAULT_MISSION_CANVAS_CONFIG, selectMissionCanvasRenderer } from "./mission-canvas-config";

describe("selectMissionCanvasRenderer", () => {
	it("defaults to the accessible list renderer with no config", () => {
		expect(selectMissionCanvasRenderer()).toBe("list");
		expect(DEFAULT_MISSION_CANVAS_CONFIG.renderer).toBe("list");
	});

	it("only switches to flow on an explicit request", () => {
		expect(selectMissionCanvasRenderer({ renderer: "flow" })).toBe("flow");
	});

	it("degrades an unknown or malformed value to list instead of throwing", () => {
		expect(selectMissionCanvasRenderer({ renderer: "surprise" as never })).toBe("list");
		expect(selectMissionCanvasRenderer({})).toBe("list");
	});
});
