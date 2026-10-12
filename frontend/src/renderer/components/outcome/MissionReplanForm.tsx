import { useState } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { apiClient } from "../../lib/api-client";
import { classifyOutcomeFailure, outcomeQueryKey, planQueryKey } from "../../hooks/useOutcome";
import { StateMark } from "@pin4sf/kennel-product-ui";
import { Button } from "../ui/button";

export function MissionReplanForm({ outcomeId, revision }: { outcomeId: string; revision: number }) {
	const { t } = useTranslation();
	const cache = useQueryClient();
	const [feedback, setFeedback] = useState("");
	const mutation = useMutation({
		retry: false,
		mutationFn: async () => {
			const { data, error } = await apiClient.POST("/api/v1/outcomes/{outcomeId}/plans/replan", {
				params: { path: { outcomeId } },
				body: { expectedContractRevision: revision, feedback },
			});
			if (error) throw error;
			return data;
		},
		onSuccess: () => {
			// Refetch current facts instead of installing a possibly delayed response.
			void cache.invalidateQueries({ queryKey: planQueryKey(outcomeId) });
			void cache.invalidateQueries({ queryKey: outcomeQueryKey(outcomeId) });
			setFeedback("");
		},
	});
	return (
		<form
			className="mt-4 flex flex-col gap-2 hairline-t border-wd-hairline pt-4"
			onSubmit={(event) => {
				event.preventDefault();
				if (feedback.trim() && !mutation.isPending && !mutation.error) mutation.mutate();
			}}
		>
			<label className="flex flex-col gap-2 text-sm font-medium text-wd-primary">
				{t("mission.replanFeedback")}
				<textarea
					className="block min-h-20 w-full rounded-md bg-wd-inset p-3 text-sm font-normal text-wd-primary outline-none placeholder:text-wd-tertiary focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-(--border-focus) disabled:text-wd-disabled"
					value={feedback}
					disabled={mutation.isPending}
					onChange={(event) => setFeedback(event.target.value)}
				/>
			</label>
			<p className="text-xs tabular-nums text-wd-secondary">{t("mission.replanNote", { revision })}</p>
			<Button className="self-start" size="sm" disabled={!feedback.trim() || mutation.isPending || Boolean(mutation.error)} type="submit">
				{t(mutation.isPending ? "mission.replanning" : "mission.replan")}
			</Button>
			{mutation.error && (
				<div role="alert" className="flex flex-col gap-1 rounded-md bg-wd-inset p-3 text-xs">
					<p className="flex items-start gap-1.5 text-wd-primary">
						<StateMark className="mt-0.5 text-critical" tone="critical" />
						{classifyOutcomeFailure(mutation.error).message}
					</p>
					<p className="text-wd-secondary">{t("mission.replanReconcile")}</p>
					<Button
						className="mt-1 self-start"
						size="sm"
						variant="secondary"
						onClick={() => {
							void cache.invalidateQueries({ queryKey: planQueryKey(outcomeId) });
							void cache.invalidateQueries({ queryKey: outcomeQueryKey(outcomeId) });
						}}
					>
						{t("mission.refresh")}
					</Button>
				</div>
			)}
		</form>
	);
}
