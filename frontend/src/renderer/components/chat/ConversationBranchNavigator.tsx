import { ChevronLeft, ChevronRight } from "lucide-react";
import { useTranslation } from "react-i18next";
import type { ConversationBranchPoint } from "../../types/conversation";

export function ConversationBranchNavigator({
	point,
	pending,
	error,
	onActivate,
}: {
	point: ConversationBranchPoint;
	pending: boolean;
	error?: string;
	onActivate: (branchId: string) => Promise<unknown> | void;
}) {
	const { t } = useTranslation();
	if (point.total <= 1) return null;
	return (
		<div className="flex min-w-0 items-center gap-0.5 text-[11px] text-wd-secondary">
		{point.previousBranchId ? (
			<button
				type="button"
				disabled={pending}
				onClick={() => {
					void Promise.resolve(onActivate(point.previousBranchId as string)).catch(() => {});
				}}
				aria-label={t("chat.branch.previous")}
				title={t("chat.branch.previous")}
				className="rounded-full p-0.5 transition-colors duration-fast hover:bg-wd-hover hover:text-wd-primary disabled:text-wd-disabled"
			>
				<ChevronLeft aria-hidden="true" className="size-3.5" />
			</button>
		) : null}
		<span
			className="wd-data-s px-0.5"
			aria-label={t("chat.branch.position", { position: point.position, total: point.total })}
		>
			{point.position} / {point.total}
		</span>
		{point.nextBranchId ? (
			<button
				type="button"
				disabled={pending}
				onClick={() => {
					void Promise.resolve(onActivate(point.nextBranchId as string)).catch(() => {});
				}}
				aria-label={t("chat.branch.next")}
				title={t("chat.branch.next")}
				className="rounded-full p-0.5 transition-colors duration-fast hover:bg-wd-hover hover:text-wd-primary disabled:text-wd-disabled"
			>
				<ChevronRight aria-hidden="true" className="size-3.5" />
			</button>
		) : null}
		{error ? <span role="alert" className="ml-1 truncate text-critical">{error}</span> : null}
	</div>
	);
}
