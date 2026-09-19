import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useRef } from "react";

import type { components } from "../../api/schema";
import { apiClient, apiErrorCode } from "../lib/api-client";
import { classifyOutcomeFailure, type OutcomeFailure } from "./useOutcome";

export type OutcomeDeliveryRecord = components["schemas"]["ControllersOutcomeDeliveryResponse"];
export type OutcomeDocumentContext = components["schemas"]["ControllersOutcomeDocumentContextResponse"];
export type RequestOutcomeDeliveryInput = Omit<
	components["schemas"]["ControllersRequestOutcomeDeliveryRequest"],
	"requestKey"
>;

type DeliveriesEnvelope = components["schemas"]["ControllersOutcomeDeliveriesEnvelope"];
type DeliveryEnvelope = components["schemas"]["ControllersOutcomeDeliveryEnvelope"];
type DocumentContextEnvelope = components["schemas"]["ControllersOutcomeDocumentContextEnvelope"];
type SelectDocumentsRequest = components["schemas"]["ControllersSelectOutcomeDocumentsRequest"];
type ApproveDocumentsRequest = components["schemas"]["ControllersApproveOutcomeDocumentsRequest"];

export const outcomeDeliveriesQueryKey = (outcomeId: string | undefined) => ["outcome-deliveries", outcomeId ?? ""] as const;
export const outcomeDocumentsQueryKey = (outcomeId: string | undefined) => ["outcome-documents", outcomeId ?? ""] as const;

export function useOutcomeDeliveries(outcomeId: string | undefined) {
	const query = useQuery({
		queryKey: outcomeDeliveriesQueryKey(outcomeId),
		enabled: Boolean(outcomeId),
		retry: false,
		queryFn: async () => {
			const { data, error } = await apiClient.GET("/api/v1/outcomes/{outcomeId}/deliveries", {
				params: { path: { outcomeId: outcomeId as string } },
			});
			if (error) throw error;
			return (data as DeliveriesEnvelope).deliveries ?? [];
		},
	});
	return {
		deliveries: query.data ?? [],
		isLoading: query.isLoading,
		failure: query.error ? classifyOutcomeFailure(query.error) : undefined,
		refetch: () => void query.refetch(),
	};
}

/**
 * Delivery is an owner-triggered, exact-artifact write. The request key lives
 * in this hook so a transport retry replays the same delivery identity rather
 * than creating a second filesystem effect.
 */
export function useRequestOutcomeDelivery(outcomeId: string | undefined) {
	const queryClient = useQueryClient();
	const requestKey = useRef<{ fingerprint: string; value: string } | undefined>(undefined);
	const mutation = useMutation({
		mutationFn: async (input: RequestOutcomeDeliveryInput) => {
			const fingerprint = JSON.stringify(input);
			if (requestKey.current?.fingerprint !== fingerprint) {
				requestKey.current = { fingerprint, value: crypto.randomUUID() };
			}
			const { data, error } = await apiClient.POST("/api/v1/outcomes/{outcomeId}/deliveries", {
				params: { path: { outcomeId: outcomeId as string } },
				body: { ...input, requestKey: requestKey.current.value },
			});
			if (error) throw error;
			return (data as DeliveryEnvelope).delivery;
		},
		onSuccess: (delivery) => {
			requestKey.current = undefined;
			queryClient.setQueryData<OutcomeDeliveryRecord[]>(outcomeDeliveriesQueryKey(outcomeId), (current = []) => {
				const withoutReplay = current.filter((item) => item.id !== delivery.id);
				return [delivery, ...withoutReplay];
			});
		},
		onSettled: () => {
			void queryClient.invalidateQueries({ queryKey: outcomeDeliveriesQueryKey(outcomeId) });
		},
	});
	return {
		pending: mutation.isPending,
		failure: mutation.error ? classifyOutcomeFailure(mutation.error) : undefined,
		reset: () => mutation.reset(),
		request: mutation.mutateAsync,
	};
}

export function useOutcomeDocumentContext(outcomeId: string | undefined) {
	const query = useQuery({
		queryKey: outcomeDocumentsQueryKey(outcomeId),
		enabled: Boolean(outcomeId),
		retry: false,
		queryFn: async () => {
			const { data, error } = await apiClient.GET("/api/v1/outcomes/{outcomeId}/documents", {
				params: { path: { outcomeId: outcomeId as string } },
			});
			if (error) {
				// No selection is the normal state for repository work: the daemon
				// answers DOCUMENT_CONTEXT_STALE. That is empty, not a failure -
				// real selection and approval errors keep surfacing below.
				if (apiErrorCode(error) === "DOCUMENT_CONTEXT_STALE") return null;
				throw error;
			}
			return (data as DocumentContextEnvelope).documentContext;
		},
	});
	return {
		context: query.data,
		isLoading: query.isLoading,
		failure: query.error ? classifyOutcomeFailure(query.error) : undefined,
		refetch: () => void query.refetch(),
	};
}

export function useSelectOutcomeDocuments(outcomeId: string | undefined) {
	const queryClient = useQueryClient();
	const mutation = useMutation({
		mutationFn: async (input: SelectDocumentsRequest) => {
			const { data, error } = await apiClient.POST("/api/v1/outcomes/{outcomeId}/documents", {
				params: { path: { outcomeId: outcomeId as string } },
				body: input,
			});
			if (error) throw error;
			return (data as DocumentContextEnvelope).documentContext;
		},
		onSuccess: (context) => {
			queryClient.setQueryData(outcomeDocumentsQueryKey(outcomeId), context);
		},
	});
	return {
		pending: mutation.isPending,
		failure: mutation.error ? classifyOutcomeFailure(mutation.error) : undefined,
		reset: () => mutation.reset(),
		select: mutation.mutateAsync,
	};
}

export function useApproveOutcomeDocuments(outcomeId: string | undefined) {
	const queryClient = useQueryClient();
	const mutation = useMutation({
		mutationFn: async (input: ApproveDocumentsRequest) => {
			const { data, error } = await apiClient.POST("/api/v1/outcomes/{outcomeId}/documents/approval", {
				params: { path: { outcomeId: outcomeId as string } },
				body: input,
			});
			if (error) throw error;
			return (data as DocumentContextEnvelope).documentContext;
		},
		onSuccess: (context) => {
			queryClient.setQueryData(outcomeDocumentsQueryKey(outcomeId), context);
		},
	});
	return {
		pending: mutation.isPending,
		failure: mutation.error ? classifyOutcomeFailure(mutation.error) : undefined,
		reset: () => mutation.reset(),
		approve: mutation.mutateAsync,
	};
}

export type OutcomeArtifactFailure = OutcomeFailure;
