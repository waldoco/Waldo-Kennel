import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import {
	Background,
	Controls,
	Handle,
	Position,
	ReactFlow,
	ReactFlowProvider,
	type Edge,
	type Node,
	type NodeProps,
	type ReactFlowInstance,
} from "@xyflow/react";
import "@xyflow/react/dist/style.css";

import {
	buildMissionCanvasEdges,
	layerMissionCanvasFixture,
	type MissionCanvasFixtureNode,
} from "../../lib/mission-canvas-fixture";
import { selectMissionCanvasRenderer, type MissionCanvasConfig } from "../../lib/mission-canvas-config";
import { cn } from "../../lib/utils";
import { resolveTheme } from "../../lib/theme";
import { useUiStore } from "../../stores/ui-store";

/**
 * F0 spike: evaluates @xyflow/react against the direct-Outcome execution
 * graph's shape (dependency-layered WorkUnits), behind the
 * mission-canvas-config boundary.
 *
 * This is deliberately NOT a replacement for MissionWorkUnitGraph. It reads a
 * static fixture, never a live schedule, and its only callback is selection —
 * there is no prop through which it could execute, mutate, or authorize
 * anything. Outside this file and its fixture, nothing constructs a "flow"
 * config, so the production graph is unaffected by this file existing.
 */

const STATE_LABEL: Record<string, string> = {
	proven: "Proven",
	executing: "Executing",
	paused: "Paused",
	retryable: "Retryable",
	blocked: "Blocked",
	runnable: "Runnable",
};

const STATE_TONE: Record<string, string> = {
	proven: "border-l-status-ready",
	executing: "border-l-status-working",
	paused: "border-l-status-needs-you",
	retryable: "border-l-status-needs-you",
	blocked: "border-l-border-strong",
	runnable: "border-l-accent",
};

function nodeAriaLabel(node: MissionCanvasFixtureNode): string {
	const stateLabel = STATE_LABEL[node.state] ?? node.state;
	const blocker = node.blockedReason ? `. ${node.blockedReason.replace(/_/g, " ")}` : "";
	return `${node.title} — ${stateLabel}${blocker}`;
}

function nodeFaceClassName(node: MissionCanvasFixtureNode, selected: boolean, extra?: string): string {
	return cn(
		"flex flex-col gap-0.5 rounded-md hairline border-border bg-card px-3 py-2 border-l-2 text-left",
		"focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring/70",
		STATE_TONE[node.state] ?? "border-l-border",
		selected && "ring-1 ring-ring/40",
		extra,
	);
}

function usePrefersReducedMotion(): boolean {
	const [reduced, setReduced] = useState(
		() => typeof window !== "undefined" && Boolean(window.matchMedia?.("(prefers-reduced-motion: reduce)").matches),
	);
	useEffect(() => {
		if (typeof window === "undefined" || !window.matchMedia) return;
		const query = window.matchMedia("(prefers-reduced-motion: reduce)");
		const listener = () => setReduced(query.matches);
		query.addEventListener("change", listener);
		return () => query.removeEventListener("change", listener);
	}, []);
	return reduced;
}

type FlowNodeData = {
	node: MissionCanvasFixtureNode;
	selected: boolean;
};

function MissionCanvasNodeFace({ data }: NodeProps<Node<FlowNodeData>>) {
	const { node, selected } = data;
	return (
		<>
			{/* Anchor points only — routing edges needs a handle position even
			    though nothing here is user-connectable (nodesConnectable={false}). */}
			<Handle className="opacity-0" position={Position.Left} type="target" />
			<button
				aria-current={selected ? "true" : undefined}
				aria-label={nodeAriaLabel(node)}
				className={nodeFaceClassName(node, selected, "w-56")}
				data-selected={selected || undefined}
				data-state={node.state}
				data-testid="mission-canvas-flow-node"
				tabIndex={0}
				type="button"
			>
				<span className="truncate text-xs text-foreground">{node.title}</span>
				<span className="text-2xs text-passive">{STATE_LABEL[node.state] ?? node.state}</span>
			</button>
			<Handle className="opacity-0" position={Position.Right} type="source" />
		</>
	);
}

const NODE_TYPES = { mission: MissionCanvasNodeFace };
const LAYER_X_SPACING = 240;
const NODE_Y_SPACING = 72;

export type MissionCanvasSpikeProps = {
	nodes: MissionCanvasFixtureNode[];
	/** Bump only on an authorized topology swap. A state-only refresh must reuse the same revision. */
	revision: number | string;
	config?: MissionCanvasConfig;
	selectedId?: string;
	onSelectNode?: (id: string) => void;
	/** Spike/test-only hook: exposes the live ReactFlowInstance so a test can spy on `fitView`. Never used by production callers. */
	onInstanceReady?: (instance: ReactFlowInstance<Node<FlowNodeData>, Edge>) => void;
};

export function MissionCanvasSpike({ nodes, revision, config, selectedId, onSelectNode, onInstanceReady }: MissionCanvasSpikeProps) {
	const renderer = selectMissionCanvasRenderer(config);
	const levels = useMemo(() => layerMissionCanvasFixture(nodes), [nodes]);

	if (levels.length === 0) return null;

	if (renderer !== "flow") {
		return <MissionCanvasListFallback levels={levels} onSelectNode={onSelectNode} selectedId={selectedId} />;
	}

	return (
		<ReactFlowProvider>
			<MissionCanvasFlow
				levels={levels}
				onInstanceReady={onInstanceReady}
				onSelectNode={onSelectNode}
				revision={revision}
				selectedId={selectedId}
			/>
		</ReactFlowProvider>
	);
}

function MissionCanvasFlow({
	levels,
	revision,
	selectedId,
	onSelectNode,
	onInstanceReady,
}: {
	levels: MissionCanvasFixtureNode[][];
	revision: number | string;
	selectedId?: string;
	onSelectNode?: (id: string) => void;
	onInstanceReady?: (instance: ReactFlowInstance<Node<FlowNodeData>, Edge>) => void;
}) {
	const instanceRef = useRef<ReactFlowInstance<Node<FlowNodeData>, Edge> | null>(null);
	const previousRevisionRef = useRef(revision);
	const reducedMotion = usePrefersReducedMotion();

	const flat = useMemo(() => levels.flat(), [levels]);

	const flowNodes = useMemo<Node<FlowNodeData>[]>(
		() =>
			levels.flatMap((level, layerIndex) =>
				level.map((node, indexInLayer) => ({
					id: node.id,
					type: "mission",
					position: { x: layerIndex * LAYER_X_SPACING, y: indexInLayer * NODE_Y_SPACING },
					data: { node, selected: node.id === selectedId },
					draggable: false,
					connectable: false,
					focusable: false,
				})),
			),
		[levels, selectedId],
	);

	const flowEdges = useMemo<Edge[]>(
		() =>
			buildMissionCanvasEdges(flat).map((edge) => ({
				id: edge.id,
				source: edge.source,
				target: edge.target,
				type: "smoothstep",
				focusable: false,
				selectable: false,
			})),
		[flat],
	);

	// State-only updates (a node's `data` changing) must never re-fit the
	// viewport — only an authorized topology swap (a new revision) may.
	useEffect(() => {
		if (instanceRef.current && previousRevisionRef.current !== revision) {
			instanceRef.current.fitView({ duration: reducedMotion ? 0 : 200 });
		}
		previousRevisionRef.current = revision;
	}, [revision, reducedMotion]);

	const handleInit = useCallback(
		(instance: ReactFlowInstance<Node<FlowNodeData>, Edge>) => {
			instanceRef.current = instance;
			onInstanceReady?.(instance);
		},
		[onInstanceReady],
	);

	const handleNodeClick = useCallback(
		(_event: unknown, node: { id: string }) => {
			onSelectNode?.(node.id);
		},
		[onSelectNode],
	);

	// React Flow's own chrome (Controls, attribution) ships a light default
	// surface; bind it to the resolved app theme so the controls stay visible
	// in dark mode without breaking light.
	const themePreference = useUiStore((state) => state.themePreference);
	const colorMode = resolveTheme(themePreference);

	return (
		<div data-testid="mission-canvas-flow" style={{ height: 480, width: "100%" }}>
			<ReactFlow
				colorMode={colorMode}
				edges={flowEdges}
				elementsSelectable={false}
				fitView
				fitViewOptions={{ duration: reducedMotion ? 0 : 200 }}
				nodes={flowNodes}
				nodesConnectable={false}
				nodesDraggable={false}
				nodesFocusable={false}
				nodeTypes={NODE_TYPES}
				onInit={handleInit}
				onNodeClick={handleNodeClick}
			>
				<Background gap={24} />
				<Controls showInteractive={false} />
			</ReactFlow>
		</div>
	);
}

function MissionCanvasListFallback({
	levels,
	selectedId,
	onSelectNode,
}: {
	levels: MissionCanvasFixtureNode[][];
	selectedId?: string;
	onSelectNode?: (id: string) => void;
}) {
	const nodeRefs = useRef(new Map<string, HTMLButtonElement>());
	const flat = useMemo(() => levels.flat(), [levels]);
	const selected = selectedId && flat.some((node) => node.id === selectedId) ? selectedId : undefined;

	const move = useCallback(
		(from: string, delta: number) => {
			const index = flat.findIndex((node) => node.id === from);
			if (index < 0) return;
			const next = flat[Math.min(Math.max(index + delta, 0), flat.length - 1)];
			if (!next) return;
			onSelectNode?.(next.id);
			nodeRefs.current.get(next.id)?.focus();
		},
		[flat, onSelectNode],
	);

	return (
		<ol className="flex min-w-0 flex-col gap-1.5" data-testid="mission-canvas-list">
			{levels.map((level, index) => (
				<li className="flex flex-col gap-1.5" key={index}>
					<p className="text-2xs uppercase tracking-wide text-passive">{index === 0 ? "Starts first" : `Step ${index + 1}`}</p>
					<div className="flex flex-wrap gap-1.5">
						{level.map((node) => {
							const isSelected = selected === node.id;
							return (
								<button
									aria-current={isSelected ? "true" : undefined}
									aria-label={nodeAriaLabel(node)}
									className={nodeFaceClassName(node, isSelected, "min-w-0 max-w-72")}
									data-state={node.state}
									data-testid="mission-canvas-node"
									key={node.id}
									onClick={() => onSelectNode?.(node.id)}
									onKeyDown={(event) => {
										if (event.key === "ArrowRight" || event.key === "ArrowDown") {
											event.preventDefault();
											move(node.id, 1);
										} else if (event.key === "ArrowLeft" || event.key === "ArrowUp") {
											event.preventDefault();
											move(node.id, -1);
										}
									}}
									ref={(element) => {
										if (element) nodeRefs.current.set(node.id, element);
										else nodeRefs.current.delete(node.id);
									}}
									tabIndex={selected ? (isSelected ? 0 : -1) : flat[0]?.id === node.id ? 0 : -1}
									type="button"
								>
									<span className="truncate text-xs text-foreground">{node.title}</span>
									<span className="text-2xs text-passive">{STATE_LABEL[node.state] ?? node.state}</span>
								</button>
							);
						})}
					</div>
				</li>
			))}
		</ol>
	);
}
