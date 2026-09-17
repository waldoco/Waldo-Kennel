/**
 * The one explicit boundary through which a React Flow-based renderer could
 * ever be selected. Nothing outside the spike fixture constructs a config
 * with `renderer: "flow"`, so this file cannot silently replace the
 * production graph (MissionWorkUnitGraph / DecompositionGraph) — those
 * components don't read this config at all.
 */
export type MissionCanvasRenderer = "list" | "flow";

export type MissionCanvasConfig = {
	renderer: MissionCanvasRenderer;
};

export const DEFAULT_MISSION_CANVAS_CONFIG: MissionCanvasConfig = { renderer: "list" };

/**
 * Resolves the renderer to use, falling back to the ordered-list renderer for
 * anything other than an explicit "flow" request. A malformed or unknown
 * value degrades to the accessible fallback rather than throwing.
 */
export function selectMissionCanvasRenderer(config?: Partial<MissionCanvasConfig>): MissionCanvasRenderer {
	return config?.renderer === "flow" ? "flow" : "list";
}
