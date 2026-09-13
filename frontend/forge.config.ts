import type { ForgeConfig } from "@electron-forge/shared-types";
import { VitePlugin } from "@electron-forge/plugin-vite";
import MakerNSIS from "./makers/maker-nsis";
import MakerDMG, { sealDmg, verifyDmg } from "./makers/maker-dmg";
import MakerAppImage from "./makers/maker-appimage";
import { writeFileSync } from "node:fs";
import { execFileSync } from "node:child_process";
import path from "node:path";
import {
	APP_ID,
	AUTH_PROTOCOL as AUTH_PROTOCOL_SCHEME,
	EXECUTABLE_NAME,
	PRODUCT_NAME,
	RELEASE_REPOSITORY,
	UPDATER_CACHE_DIRECTORY_NAME,
} from "./src/shared/product-identity";

// Default GitHub release target (production): Kennel's own repository, which
// is the single source of truth in product-identity. Builds cut by CI must NOT
// rely on this fallback: the
// workflows set KENNEL_RELEASE_REPO to the repo they run in, and the package
// asserts the baked app-update.yml matches it, so a future org/repo rename
// fails the build instead of stranding the fleet on a redirect (#3523).
const DEFAULT_RELEASE_REPO = RELEASE_REPOSITORY;

// The packaged binary name (no extension). Single source of truth: the packager
// names the exe/ELF from this, and the NSIS + deb makers must point their
// shortcut/launcher at the SAME name. Drift here means a broken Start menu
// shortcut on Windows (#2414) or "could not find the Electron app binary" on deb.
const AUTH_PROTOCOL = {
	name: "Kennel authentication callback",
	schemes: [AUTH_PROTOCOL_SCHEME],
};
const AUTH_PROTOCOL_MIME_TYPE = `x-scheme-handler/${AUTH_PROTOCOL_SCHEME}`;

// parseReleaseRepo turns an "owner/repo" string (from KENNEL_RELEASE_REPO) into the
// publisher-github { owner, name } shape, falling back to the production default
// when unset or malformed.
function parseReleaseRepo(value: string | undefined): { owner: string; name: string } {
	const [owner, name] = (value || DEFAULT_RELEASE_REPO).split("/");
	if (!owner || !name) {
		const [defOwner, defName] = DEFAULT_RELEASE_REPO.split("/");
		return { owner: defOwner, name: defName };
	}
	return { owner, name };
}

const config: ForgeConfig = {
	packagerConfig: {
		asar: true,
		appBundleId: APP_ID,
		name: PRODUCT_NAME,
		executableName: EXECUTABLE_NAME,
		protocols: [AUTH_PROTOCOL],
		appCategoryType: "public.app-category.developer-tools",
		// App icon. electron-packager appends the per-platform extension
		// (.icns on macOS, .ico on Windows); Linux menu icons come from the
		// deb/rpm makers below, and the runtime window icon from src/main.ts.
		icon: "assets/icon",
		extraResource: [
			"daemon",
			"agent-browser",
			"resources/acp-runtime",
			"../packages/kennel-island/desktop/helpers/kennel-haptics",
			"../packages/kennel-island/desktop/helpers/kennel-notch",
			"assets/icon.png",
			"assets/icon.ico",
			"assets/trayIconTemplate.png",
			"assets/trayIconTemplate@2x.png",
			"app-update.yml",
		],
		// electron-packager derives CFBundleDisplayName from executableName after
		// applying extendInfo. Kennel deliberately uses a lowercase CLI executable,
		// so set the Finder-facing name after resources are copied but before the
		// signing/notarization stages seal the bundle.
		afterCopyExtraResources: [
			(buildPath, _electronVersion, platform, _arch, callback) => {
				if (platform !== "darwin") {
					callback();
					return;
				}
				try {
					execFileSync("/usr/bin/plutil", [
						"-replace",
						"CFBundleDisplayName",
						"-string",
						PRODUCT_NAME,
						path.join(buildPath, `${PRODUCT_NAME}.app`, "Contents", "Info.plist"),
					]);
					callback();
				} catch (error) {
					callback(error instanceof Error ? error : new Error(String(error)));
				}
			},
		],
		// Notarization. Two paths:
		//  - CI: an App Store Connect API key. APPLE_API_KEY is a PATH to the .p8
		//    (the workflow decodes APPLE_API_KEY_BASE64 to a temp file), plus the
		//    key id + issuer uuid. Matches the proven local runbook creds.
		//  - Local: KENNEL_NOTARY_PROFILE, a notarytool keychain profile created with
		//    `notarytool store-credentials`. See ao-macos-signed-release runbook.
		// Both are valid NotaryToolCredentials, so no cast is needed.
		osxSign: process.env.APPLE_SIGNING_IDENTITY
			? { identity: process.env.APPLE_SIGNING_IDENTITY }
			: process.env.CSC_LINK
				? {}
				: undefined,
		osxNotarize: process.env.KENNEL_NOTARY_PROFILE
			? { keychainProfile: process.env.KENNEL_NOTARY_PROFILE }
			: process.env.APPLE_API_KEY
				? {
						appleApiKey: process.env.APPLE_API_KEY,
						appleApiKeyId: process.env.APPLE_API_KEY_ID!,
						appleApiIssuer: process.env.APPLE_API_ISSUER!,
					}
				: undefined,
	},
	hooks: {
		// electron-forge does not generate app-update.yml (electron-builder does);
		// electron-updater reads it from the app's Resources dir at runtime to know
		// which GitHub repo to pull from, else it throws ENOENT during download.
		// Generate it in prePackage (BEFORE osxSign) and ship it via extraResource
		// above, so it is copied into the bundle and SIGNED as part of the seal.
		// Writing it after signing (a postPackage hook) adds an unsealed resource
		// and macOS reports the app as "damaged". owner/repo are baked from
		// KENNEL_RELEASE_REPO at build time.
		prePackage: async () => {
			// The native helpers are best-effort: the script deliberately succeeds on
			// non-macOS hosts and on Macs without a Swift toolchain. When available,
			// Forge copies the resulting binaries beside app.asar via extraResource.
			execFileSync(process.execPath, ["../packages/kennel-island/scripts/build-helpers.mjs"], {
				stdio: "inherit",
			});
			const { owner, name } = parseReleaseRepo(process.env.KENNEL_RELEASE_REPO);
			const yml = [
				"provider: github",
				`owner: ${owner}`,
				`repo: ${name}`,
				`updaterCacheDirName: ${UPDATER_CACHE_DIRECTORY_NAME}`,
				"",
			].join("\n");
			writeFileSync("app-update.yml", yml);
		},
		// The dmg container is NOT signed, notarized or stapled by any maker
		// (neither Forge's maker-dmg nor app-builder-lib's dmg target does it), and
		// the .app's own stapled ticket does not propagate through an unsealed
		// container. So seal it here, after the maker has produced it, reusing the
		// same credentials packagerConfig already consumes (#3267 decision 3).
		// The .app inside was already signed + notarized + stapled by
		// packagerConfig above, before any maker ran; nothing here touches it.
		//
		// Then PROVE the seal. sealDmg exiting 0 only says three commands ran on
		// this machine; it does not say Gatekeeper accepts the published bytes with
		// a stapled ticket. verify-mac-artifact.sh is the canonical gate for that
		// (#3288 workstreams 1 and 2), and #3267 decision 3 step 4 asks for exactly
		// this check on the dmg. Run only when sealDmg actually sealed: an unsigned
		// local or desktop-testing build has nothing to verify and must keep
		// producing its dmg.
		postMake: async (_forgeConfig, makeResults) => {
			for (const result of makeResults) {
				if (result.platform !== "darwin") continue;
				for (const artifact of result.artifacts) {
					if (!artifact.endsWith(".dmg")) continue;
					if (await sealDmg(artifact)) await verifyDmg(artifact);
				}
			}
			return makeResults;
		},
	},
	rebuildConfig: {},
	makers: [
		// Windows installer: NSIS via electron-builder (see makers/maker-nsis.ts).
		// Replaces Squirrel.Windows, which only does per-user installs with no
		// custom install dir or proper uninstaller (issue #401).
		new MakerNSIS(
			{
				appId: APP_ID,
				productName: PRODUCT_NAME,
				// Match the packaged binary name so the Start menu shortcut targets
				// the real "kennel.exe" (not "Kennel.exe").
				executableName: EXECUTABLE_NAME,
				icon: "assets/icon.ico",
			},
			["win32"],
		),
		// macOS auto-update artifact. This entry can NEVER be removed:
		// MacUpdater.doDownloadUpdate looks for a "zip" and explicitly excludes
		// .pkg/.dmg, throwing ERR_UPDATER_ZIP_FILE_NOT_FOUND otherwise, so the zip
		// and latest-mac.yml must keep publishing forever (#3267 decision 2).
		{ name: "@electron-forge/maker-zip", platforms: ["darwin"], config: {} },
		// macOS FIRST-INSTALL artifact, additive to the zip above: a dmg has no
		// user-driven extraction step, so a third-party unzip tool can no longer
		// break the signature seal on the way in (see makers/maker-dmg.ts, #3267).
		new MakerDMG(
			{
				appId: APP_ID,
				productName: PRODUCT_NAME,
			},
			["darwin"],
		),
		// Linux fetch-and-run artifact for `kennel start`: a single self-contained
		// AppImage the Go bootstrapper downloads and runs directly (see
		// makers/maker-appimage.ts). The deb/rpm makers below stay for users who
		// prefer a system package.
		new MakerAppImage(
			{
				appId: APP_ID,
				productName: PRODUCT_NAME,
				icon: "assets/icon.png",
				protocols: [AUTH_PROTOCOL],
			},
			["linux"],
		),
		{
			name: "@electron-forge/maker-deb",
			config: {
				options: {
					// Must match packagerConfig.executableName, or the deb maker
					// looks for the package name and fails with "could not find
					// the Electron app binary". (Both are "kennel".)
					bin: EXECUTABLE_NAME,
					icon: "assets/icon.png",
					maintainer: PRODUCT_NAME,
					homepage: "https://github.com/waldoco/Waldo-Kennel",
					mimeType: [AUTH_PROTOCOL_MIME_TYPE],
				},
			},
		},
		{
			name: "@electron-forge/maker-rpm",
			config: {
				options: {
					icon: "assets/icon.png",
					// rpmbuild rejects a spec with an empty License field.
					license: "Apache-2.0",
					homepage: "https://github.com/waldoco/Waldo-Kennel",
					mimeType: [AUTH_PROTOCOL_MIME_TYPE],
				},
			},
		},
	],
	publishers: [
		{
			name: "@electron-forge/publisher-github",
			// Release target is build-time overridable so a fork run publishes to the
			// fork without a source edit. KENNEL_RELEASE_REPO is "owner/repo"; it defaults
			// to the production target. The dev/test loop sets
			// Release builds must explicitly remain on the Kennel-owned repository.
			config: {
				repository: parseReleaseRepo(process.env.KENNEL_RELEASE_REPO),
				prerelease: process.env.KENNEL_RELEASE_PRERELEASE === "true",
				draft: false,
			},
		},
	],
	plugins: [
		new VitePlugin({
			build: [
				{ entry: "src/main.ts", config: "vite.main.config.ts", target: "main" },
				{ entry: "src/preload.ts", config: "vite.preload.config.ts", target: "preload" },
				{ entry: "src/island-preload.ts", config: "vite.preload.config.ts", target: "preload" },
				{ entry: "src/annotate-preload.ts", config: "vite.preload.config.ts", target: "preload" },
			],
			renderer: [
				{ name: "main_window", config: "vite.renderer.config.ts" },
				{ name: "island_window", config: "vite.island.renderer.config.ts" },
			],
		}),
	],
};

export default config;
