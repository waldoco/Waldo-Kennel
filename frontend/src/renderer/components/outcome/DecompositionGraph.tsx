import { StateLabel, type StateTone } from "@pin4sf/kennel-product-ui";
import { useMemo } from "react";
import { useTranslation } from "react-i18next";

import type { components } from "../../../api/schema";
import { cn } from "../../lib/utils";
import { layerByDependency } from "../../lib/dependency-layers";

type Decomposition = components["schemas"]["DecompositionResponse"];
type Contributor = components["schemas"]["ContributorResponse"];

/**
 * One contributor placed in the dependency order.
 *
 * `ref` is the decomposition's own stable handle for a contribution; the
 * Outcome id only exists once the decomposition is authorized, so the graph is
 * keyed by ref and enriches with live state when there is any.
 */
type Node = {
	/**
	 * Graph identity, equal to `ref`. Named `id` to satisfy the shared
	 * DependencyNode contract; `ref` is kept because it is the decomposition's
	 * own vocabulary and what dependency edges are expressed in.
	 */
	id: string;
	ref: string;
	title: string;
	outcomeId?: string;
	attention?: string;
	/** Refs that must finish before this one starts. */
	upstream: string[];
	blocked: boolean;
};

/**
 * Layers this decomposition's contributions.
 *
 * Thin wrapper over the shared layering so the decomposition graph and the
 * direct WorkUnit graph cannot drift apart geometrically. Their node
 * *semantics* stay separate on purpose: these edges order contributing
 * Outcomes, not execution.
 */
export function layerContributions(nodes: Node[]): Node[][] {
	return layerByDependency(nodes);
}

/** Builds the graph's nodes from a decomposition and whatever live state exists. */
export function graphNodes(decomposition: Decomposition, contributors: Contributor[]): Node[] {
	const upstreamOf = new Map<string, string[]>();
	for (const dependency of decomposition.dependencies) {
		// FromRef must finish before ToRef starts, so ToRef is downstream.
		upstreamOf.set(dependency.toRef, [...(upstreamOf.get(dependency.toRef) ?? []), dependency.fromRef]);
	}
	return decomposition.contributors.map((contribution) => {
		const live = contribution.childOutcomeId
			? contributors.find((entry) => entry.outcome.id === contribution.childOutcomeId)
			: undefined;
		return {
			id: contribution.ref,
			ref: contribution.ref,
			title: contribution.title,
			outcomeId: contribution.childOutcomeId,
			attention: live?.attention.kind,
			upstream: upstreamOf.get(contribution.ref) ?? [],
			blocked: (live?.blockedBy.length ?? 0) > 0,
		};
	});
}

// Attention as Waldo's typed state, written beside the title with its mark -
// never a coloured rail. Only the owner's next move takes Spot; only running
// takes the live accent.
const ATTENTION_TONE: Record<string, StateTone> = {
	needs_you: "needsYou",
	action_required: "needsYou",
	ready_for_acceptance: "good",
	waiting: "neutral",
	running: "live",
	accepted: "good",
};

const ATTENTION_LABEL_KEYS: Record<string, string> = {
	needs_you: "outcome.attention.needsYou",
	action_required: "outcome.attention.actionRequired",
	ready_for_acceptance: "outcome.attention.readyForAcceptance",
	waiting: "outcome.attention.waiting",
	running: "outcome.attention.running",
	accepted: "outcome.attention.accepted",
};

/**
 * The decomposition drawn as what it is: a dependency order over contributing
 * Outcomes.
 *
 * This deliberately does not claim concurrency. The attempt fence is still
 * project-wide, so contributors serialize no matter what the dependency graph
 * permits — a picture implying "these two run together" would be drawing a
 * promise the daemon refuses to keep. What the levels DO say is what a list
 * says poorly once there are more than a few contributors: what must finish
 * before what, and where a blocked contributor sits in that order.
 */
export function DecompositionGraph({
	decomposition,
	contributors,
	onInspect,
}: {
	decomposition: Decomposition;
	contributors: Contributor[];
	onInspect?: (outcomeId: string) => void;
}) {
	const { t } = useTranslation();
	const levels = useMemo(
		() => layerContributions(graphNodes(decomposition, contributors)),
		[decomposition, contributors],
	);
	if (levels.length === 0) return null;

	return (
		<section className="flex flex-col gap-2" data-testid="decomposition-graph">
			<div>
				<h3 className="text-sm font-medium text-wd-primary">{t("outcome.graph.heading")}</h3>
				<p className="text-xs leading-body text-wd-secondary">{t("outcome.graph.serialNote")}</p>
			</div>
			<ol className="flex flex-col gap-1.5">
				{levels.map((level, index) => (
					<li className="flex flex-col gap-1.5" key={index}>
						<p className="text-xs text-wd-secondary">
							{index === 0 ? t("outcome.graph.startsFirst") : t("outcome.graph.step", { n: index + 1 })}
						</p>
						<div className="flex flex-wrap gap-1.5">
							{level.map((node) => {
								const label = node.outcomeId
									? t("outcome.graph.inspect", { title: node.title })
									: undefined;
								const body = (
									<>
										<span className="min-w-0 truncate text-sm font-medium text-wd-primary">{node.title}</span>
										{node.attention && ATTENTION_LABEL_KEYS[node.attention] ? (
											<StateLabel
												className="text-xs"
												label={t(ATTENTION_LABEL_KEYS[node.attention] as never)}
												tone={ATTENTION_TONE[node.attention] ?? "neutral"}
											/>
										) : null}
										{node.upstream.length > 0 ? (
											<span className="text-xs text-wd-tertiary">
												{t("outcome.graph.after", { refs: node.upstream.join(", ") })}
											</span>
										) : null}
										{node.blocked ? (
											<StateLabel className="text-xs" label={t("outcome.graph.blocked")} tone="caution" />
										) : null}
									</>
								);
								const className = cn(
									"flex min-w-0 max-w-64 flex-col gap-1 rounded-md hairline border-wd-hairline bg-wd-surface px-3 py-2 text-left",
								);
								// Only an authorized contribution has an Outcome to open;
								// a proposed one is still a draft and has nothing behind it.
								return node.outcomeId && onInspect ? (
									<button
										aria-label={label}
										className={cn(className, "transition-colors duration-fast hover:bg-wd-hover focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-(--border-focus) motion-reduce:transition-none")}
										key={node.ref}
										onClick={() => onInspect(node.outcomeId as string)}
										type="button"
									>
										{body}
									</button>
								) : (
									<div className={className} key={node.ref}>
										{body}
									</div>
								);
							})}
						</div>
					</li>
				))}
			</ol>
		</section>
	);
}
