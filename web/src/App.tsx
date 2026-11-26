import { Navigate, Route, Routes } from "react-router-dom";
import AppLayout from "./components/AppLayout";
import ProtectedRoute from "./components/ProtectedRoute";
import Credentials from "./pages/Credentials";
import Groups from "./pages/Groups";
import Home from "./pages/Home";
import Hosts from "./pages/Hosts";
import Login from "./pages/Login";
import Password from "./pages/Password";

export default function App() {
  return (
    <Routes>
      <Route path="/login" element={<Login />} />
      <Route element={<ProtectedRoute />}>
        <Route element={<AppLayout />}>
          <Route index element={<Home />} />
          <Route path="hosts" element={<Hosts />} />
          <Route path="hosts/groups" element={<Groups />} />
          <Route path="credentials" element={<Credentials />} />
          <Route path="password" element={<Password />} />
        </Route>
      </Route>
      <Route path="*" element={<Navigate to="/" replace />} />
    </Routes>
  );
}
