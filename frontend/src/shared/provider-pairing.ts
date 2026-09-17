export type CodexDiscoveryState =
  | {
      state: "installed";
      installationId: string;
      version: string;
      source: string;
    }
  | { state: "not_found"; message: string }
  | { state: "incompatible"; version?: string; message: string }
  | { state: "error"; message: string };

export type CodexPairingState = {
  state:
    | "unpaired"
    | "awaiting_confirmation"
    | "pairing"
    | "connected"
    | "action_needed"
    | "error";
  connectionId?: string;
  generation?: number;
  reason?: string;
  repair?: string;
  message?: string;
};

export type CodexPairingProposal = {
  projectId: string;
  installationId: string;
  requestKey: string;
};
