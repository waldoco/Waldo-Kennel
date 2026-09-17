import net from "node:net";
export function exchangePairingFrame(
  address: string,
  frame: object,
): Promise<Record<string, unknown>> {
  return new Promise((resolve, reject) => {
    const socket = net.createConnection(address);
    let pending = "",
      settled = false;
    const finish = (error?: Error, value?: Record<string, unknown>) => {
      if (settled) return;
      settled = true;
      socket.destroy();
      error ? reject(error) : resolve(value ?? {});
    };
    socket.setTimeout(5000, () => finish(Error("Pairing transport timed out")));
    socket.on("error", (e) => finish(e));
    socket.on("connect", () => socket.write(`${JSON.stringify(frame)}\n`));
    socket.on("data", (chunk) => {
      pending += chunk.toString("utf8");
      const end = pending.indexOf("\n");
      if (end < 0) return;
      try {
        const value = JSON.parse(pending.slice(0, end));
        if (!value || typeof value !== "object" || Array.isArray(value))
          throw Error("invalid frame");
        finish(undefined, value);
      } catch {
        finish(Error("Pairing transport returned an invalid frame"));
      }
    });
  });
}
