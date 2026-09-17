/**
 * A WorkUnit's proof coverage: each criterion resolved ready or not, or an
 * explicit "unavailable" reading when the projection has not resolved
 * readiness at all (`criterionReady: null`) — never presented as zero-ready.
 * Criteria are identified by position only; a raw criterion ID belongs in
 * Developer/technical details, never here.
 */
export type ContractCoverageItem = {
	position: number;
	ready: boolean;
};

export type ContractCoverageProps =
	| { unavailable: true; unavailableLabel: string }
	| { unavailable?: false; items: ContractCoverageItem[]; readyLabel: string; pendingLabel: string; criterionLabel: (position: number) => string };

export function ContractCoverage(props: ContractCoverageProps) {
	if (props.unavailable) {
		return (
			<p className="text-muted-foreground text-sm" data-testid="contract-coverage-unavailable">
				{props.unavailableLabel}
			</p>
		);
	}
	return (
		<ul className="flex flex-col gap-1" data-testid="contract-coverage-list">
			{props.items.map((item) => (
				<li className="flex items-center justify-between text-sm" key={item.position}>
					<span>{props.criterionLabel(item.position)}</span>
					<span className={item.ready ? "text-status-ready" : "text-muted-foreground"}>
						{item.ready ? props.readyLabel : props.pendingLabel}
					</span>
				</li>
			))}
		</ul>
	);
}
