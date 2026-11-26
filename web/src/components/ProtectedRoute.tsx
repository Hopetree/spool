import { useEffect } from "react";
import { Navigate, Outlet, useLocation } from "react-router-dom";
import { Spin } from "antd";
import { useAuthStore } from "../stores/auth";

export default function ProtectedRoute() {
  const status = useAuthStore((s) => s.status);
  const user = useAuthStore((s) => s.user);
  const fetchMe = useAuthStore((s) => s.fetchMe);
  const location = useLocation();

  useEffect(() => {
    if (status === "loading") {
      void fetchMe();
    }
  }, [status, fetchMe]);

  if (status === "loading") {
    return (
      <div style={{ display: "flex", height: "100vh", alignItems: "center", justifyContent: "center" }}>
        <Spin size="large" />
      </div>
    );
  }
  if (status === "anon") {
    return <Navigate to="/login" state={{ from: location.pathname }} replace />;
  }
  if (user?.must_change_password && location.pathname !== "/password") {
    return <Navigate to="/password" replace />;
  }
  return <Outlet />;
}
