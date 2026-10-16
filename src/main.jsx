import React from "react";
import ReactDOM from "react-dom/client";
import "@fontsource-variable/inter";
import "@fontsource-variable/jetbrains-mono";
import App from "./App.jsx";
import LoginPage, { NoAccessPage } from "./pages/LoginPage.jsx";
import { AuthGate } from "./lib/auth.jsx";
import "./index.css";

const root =
  window.location.pathname === "/login" ? (
    <LoginPage />
  ) : (
    <AuthGate noAccess={(me) => <NoAccessPage user={me.user} />}>
      <App />
    </AuthGate>
  );

ReactDOM.createRoot(document.getElementById("root")).render(<React.StrictMode>{root}</React.StrictMode>);
