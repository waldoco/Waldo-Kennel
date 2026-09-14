#!/usr/bin/env node

import { existsSync, readFileSync, readdirSync } from "node:fs";
import { execFileSync } from "node:child_process";
import path from "node:path";

let appPath = process.argv[2];
if (!appPath) {
	const outDir = path.resolve("out");
	const matches = existsSync(outDir)
		? readdirSync(outDir)
				.filter((name) => name.startsWith("Kennel-darwin-"))
				.map((name) => path.join(outDir, name, "Kennel.app"))
				.filter(existsSync)
		: [];
	if (matches.length !== 1) {
		throw new Error(
			`expected exactly one packaged Kennel.app under ${outDir}, found ${matches.length}; pass an explicit path`,
		);
	}
	[appPath] = matches;
}

const expected = {
	appId: "in.heywaldo.kennel",
	appName: "Kennel.app",
	executable: "kennel",
	protocol: "kennel-app",
	releaseRepo: "waldoco/Waldo-Kennel",
	updaterCache: "kennel-updater",
};

function plistValue(keyPath) {
	return execFileSync(
		"/usr/bin/plutil",
		["-extract", keyPath, "raw", "-o", "-", path.join(appPath, "Contents", "Info.plist")],
		{ encoding: "utf8" },
	).trim();
}

if (path.basename(appPath) !== expected.appName) {
	throw new Error(`bundle name is ${path.basename(appPath)}, want ${expected.appName}`);
}

const executablePath = path.join(appPath, "Contents", "MacOS", expected.executable);
if (!existsSync(executablePath)) throw new Error(`missing packaged executable ${executablePath}`);
const hookExecutablePath = path.join(appPath, "Contents", "Resources", "daemon", "kennel");
if (!existsSync(hookExecutablePath)) {
	throw new Error(`missing packaged hook CLI ${hookExecutablePath}`);
}
if (existsSync(path.join(appPath, "Contents", "MacOS", "agent-orchestrator"))) {
	throw new Error("package still contains the inherited agent-orchestrator executable");
}

const actual = {
	appId: plistValue("CFBundleIdentifier"),
	displayName: plistValue("CFBundleDisplayName"),
	executable: plistValue("CFBundleExecutable"),
	protocol: plistValue("CFBundleURLTypes.0.CFBundleURLSchemes.0"),
};
const identityExpectations = { ...expected, displayName: "Kennel" };
for (const key of Object.keys(actual)) {
	if (actual[key] !== identityExpectations[key]) {
		throw new Error(`${key} is ${actual[key]}, want ${identityExpectations[key]}`);
	}
}

const updaterPath = path.join(appPath, "Contents", "Resources", "app-update.yml");
const updater = readFileSync(updaterPath, "utf8");
for (const line of ["owner: waldoco", "repo: Waldo-Kennel", `updaterCacheDirName: ${expected.updaterCache}`]) {
	if (!updater.includes(line)) throw new Error(`${updaterPath} is missing ${JSON.stringify(line)}`);
}
if (/agent-orchestrator|ao-updater/i.test(updater)) {
	throw new Error("packaged updater metadata still targets AO identity");
}

console.log(JSON.stringify({ bundle: appPath, ...actual, releaseRepo: expected.releaseRepo, updaterCache: expected.updaterCache }));
