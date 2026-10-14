import { describe, expect, it } from "vitest";
import { pairResultToState, statusToState, validWaldoCode, validWaldoLabel } from "./waldo-connection";
import type { WaldoBridgeStatusResult } from "../../main/waldo-bridge-handler";

describe("Waldo IPC result mapping", () => {
 it("maps every status result: not ok, ready false, unknown state, unpaired, pairing, recovery_required, offline, online", () => {
  const unavailable = {kind:"unavailable"};
  for (const result of [{ok:false,reason:"failed"},{ok:false,reason:"unavailable"},{ok:true,state:"unpaired",ready:false},{ok:true,state:"future",ready:true}] as WaldoBridgeStatusResult[]) expect(statusToState(result)).toStrictEqual(unavailable);
  for (const state of ["unpaired","pairing","recovery_required"]) expect(statusToState({ok:true,state,ready:true})).toStrictEqual({kind:state});
  for (const transport of ["offline","online"]) expect(statusToState({ok:true,state:transport,ready:true,deviceId:"synthetic-device",label:"Synthetic Mac"})).toStrictEqual({kind:"paired",transport,deviceId:"synthetic-device",label:"Synthetic Mac"});
 });
 it.each(["recovery_required","unavailable","failed"] as const)("maps pair failure %s", reason => {
  expect(pairResultToState({ok:false,reason})).toStrictEqual(reason === "recovery_required" ? {kind:reason} : {kind:"error",reason});
 });
 it("never assumes pairing from an ok pair reply",()=>expect(pairResultToState({ok:true})).toStrictEqual({kind:"unavailable"}));
});

describe("Waldo boundary validation (synthetic inputs)", () => {
 it("accepts canonical unpadded 32-byte codes and rejects pad-bit aliases", () => {
  expect(validWaldoCode("A".repeat(43))).toBe(true);
  expect(validWaldoCode(`wp1.e2e00000-0000-4000-8000-000000000001.${"A".repeat(43)}`)).toBe(true);
  for (const bad of [`wp1..${"A".repeat(43)}`, `wp1.a/b.${"A".repeat(43)}`, `wp2.owner.${"A".repeat(43)}`, `wp1.owner.${"A".repeat(42)}B`, `wp1.${"o".repeat(129)}.${"A".repeat(43)}`]) expect(validWaldoCode(bad)).toBe(false);
  expect(validWaldoCode("_".repeat(42) + "8")).toBe(true);
  for (const value of ["A".repeat(42), "A".repeat(44), "A".repeat(42)+"B", "A".repeat(42)+"=", "A".repeat(42)+"/", "A".repeat(42)+"+", " A".repeat(22), "Ａ".repeat(43), "A".repeat(43)+"\n"]) expect(validWaldoCode(value)).toBe(false);
 });
 it("counts UTF-8 bytes without truncating or replacing malformed Unicode", () => {
  for (const value of ["x", "x".repeat(120), "é".repeat(60), "機".repeat(40), "😀".repeat(30)]) expect(validWaldoLabel(value)).toBe(true);
  for (const value of ["", "x".repeat(121), "é".repeat(61), "機".repeat(41), "😀".repeat(31), "\ud800", "\udc00", "x\ud800y"]) expect(validWaldoLabel(value)).toBe(false);
 });
});
