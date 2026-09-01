import { toJson } from "@bufbuild/protobuf";
import { type Value, ValueSchema } from "@bufbuild/protobuf/wkt";

/*
 * A catalog setting's `default` crosses the wire as a google.protobuf.Value
 * (the generated WorkflowSetting.default), because catalog defaults are a mix
 * of strings, numbers and booleans. This unwraps one to the plain JS value
 * the editor form and the graph's settings map work with — the browser-side
 * counterpart of the Go side reading structpb's Value.AsInterface().
 */
export function settingDefault(value: Value | undefined): unknown {
  if (value === undefined) return undefined;
  const json = toJson(ValueSchema, value);
  return json === null ? undefined : json;
}
