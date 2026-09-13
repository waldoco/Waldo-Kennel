import { describe, expect, it } from "vitest";
import config from "./forge.config";

describe("packaged Kennel identity", () => {
	it("uses Kennel-owned bundle, executable, protocol, updater, and release identities", () => {
		expect(config.packagerConfig).toMatchObject({
			appBundleId: "in.heywaldo.kennel",
			name: "Kennel",
			executableName: "kennel",
		});
		expect(config.packagerConfig?.afterCopyExtraResources).toHaveLength(1);
		expect(config.packagerConfig?.protocols).toEqual([
			{
				name: "Kennel authentication callback",
				schemes: ["kennel-app"],
			},
		]);

		const makers = config.makers as Array<{
			name?: string;
			config?: { options?: { mimeType?: string[] } };
		}>;
		for (const name of [
			"@electron-forge/maker-deb",
			"@electron-forge/maker-rpm",
		]) {
			const maker = makers.find((candidate) => candidate.name === name);
			expect(maker?.config?.options?.mimeType).toEqual([
				"x-scheme-handler/kennel-app",
			]);
		}

		const publishers = config.publishers as Array<{
			config?: { repository?: { owner?: string; name?: string } };
		}>;
		expect(publishers[0]?.config?.repository).toEqual({ owner: "waldoco", name: "Waldo-Kennel" });
	});
});
