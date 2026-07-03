import { TokenGate } from "./features/auth/TokenGate";
import { EventViewer } from "./features/events/EventViewer";

export default function App() {
  return (
    <>
      <header className="app-header">
        <h1>Ledgerly</h1>
        <p className="app-header__subtitle">
          Append-only, cryptographically verifiable audit log — read-only
          inspection surface.
        </p>
      </header>
      <main className="app-main">
        <TokenGate>
          {({ token, tenantId, resetSession }) => (
            <>
              <p className="app-main__tenant">
                Tenant <code>{tenantId}</code>
              </p>
              <EventViewer token={token} onResetSession={resetSession} />
            </>
          )}
        </TokenGate>
      </main>
    </>
  );
}
