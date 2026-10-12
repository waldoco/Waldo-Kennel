import { EvidenceRow } from "./EvidenceRow";

/**
 * A WorkUnit's proof coverage: each criterion resolved ready or not, or an
 * explicit "unavailable" reading when the projection has not resolved
 * readiness at all (`criterionReady: null`) — never presented as zero-ready.
 * Criteria are identified by position only; a raw criterion ID belongs in
 * Developer/technical details, never here.
 *
 * Each criterion is an EvidenceRow: the assertion, its verdict badge, and —
 * when the host knows it — where that verdict came from.
 */
export type ContractCoverageItem = {
	position: number;
	ready: boolean;
	/** Where the verdict came from ("Checked by Kennel · npm test · 16:40"). */
	provenance?: string;
};

export type ContractCoverageProps =
	| { unavailable: true; unavailableLabel: string }
	| {
			unavailable?: false;
			items: ContractCoverageItem[];
			readyLabel: string;
			pendingLabel: string;
			criterionLabel: (position: number) => string;
	  };

export function ContractCoverage(props: ContractCoverageProps) {
	if (props.unavailable) {
		return (
			<p className="text-sm leading-5 text-wd-secondary" data-testid="contract-coverage-unavailable">
				{props.unavailableLabel}
			</p>
		);
	}
	return (
		<ul className="flex flex-col" data-testid="contract-coverage-list">
			{props.items.map((item) => (
				<EvidenceRow
					assertion={props.criterionLabel(item.position)}
					className="py-2"
					key={item.position}
					provenance={item.provenance}
					verdict={item.ready ? "verified" : "pending"}
					verdictLabel={item.ready ? props.readyLabel : props.pendingLabel}
				/>
			))}
		</ul>
	);
}
