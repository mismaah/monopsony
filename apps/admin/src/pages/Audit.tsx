import { useEffect, useState } from "react";
import { api } from "@/lib/http";
import { Panel } from "@/lib/ui";

interface Entry {
  id: string;
  adminId: string;
  action: string;
  target: string;
  payload?: unknown;
  at: string;
}

export default function AuditPage() {
  const [entries, setEntries] = useState<Entry[]>([]);
  useEffect(() => {
    api<{ entries: Entry[] }>("GET", "/admin/api/audit").then((r) => setEntries(r.entries));
  }, []);
  return (
    <Panel title="Audit log">
      <table className="w-full text-sm">
        <thead className="text-zinc-400">
          <tr>
            <th>When</th>
            <th>Admin</th>
            <th>Action</th>
            <th>Target</th>
            <th>Payload</th>
          </tr>
        </thead>
        <tbody>
          {entries.map((e) => (
            <tr key={e.id} className="border-t border-zinc-800 align-top">
              <td className="text-zinc-400 whitespace-nowrap">{new Date(e.at).toLocaleString()}</td>
              <td className="font-mono text-xs">{e.adminId.slice(0, 8)}</td>
              <td>{e.action}</td>
              <td className="font-mono text-xs">{e.target}</td>
              <td className="font-mono text-xs text-zinc-400 max-w-md truncate">{e.payload ? JSON.stringify(e.payload) : ""}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </Panel>
  );
}
