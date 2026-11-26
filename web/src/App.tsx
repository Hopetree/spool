import { Navigate, Route, Routes } from "react-router-dom";
import AppLayout from "./components/AppLayout";
import ProtectedRoute from "./components/ProtectedRoute";
import Adhoc from "./pages/Adhoc";
import Audit from "./pages/Audit";
import Channels from "./pages/Channels";
import Credentials from "./pages/Credentials";
import ExecutionDetail from "./pages/ExecutionDetail";
import FlowEditor from "./pages/FlowEditor";
import Flows from "./pages/Flows";
import Executions from "./pages/Executions";
import Files from "./pages/Files";
import Groups from "./pages/Groups";
import Home from "./pages/Home";
import Hosts from "./pages/Hosts";
import Login from "./pages/Login";
import Password from "./pages/Password";
import ScheduleEditor from "./pages/ScheduleEditor";
import Schedules from "./pages/Schedules";
import ToolEditor from "./pages/ToolEditor";
import Tools from "./pages/Tools";
import Users from "./pages/Users";

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
          <Route path="tools" element={<Tools />} />
          <Route path="tools/new" element={<ToolEditor />} />
          <Route path="tools/:id" element={<ToolEditor />} />
          <Route path="flows" element={<Flows />} />
          <Route path="flows/new" element={<FlowEditor />} />
          <Route path="flows/:id" element={<FlowEditor />} />
          <Route path="executions" element={<Executions />} />
          <Route path="executions/:id" element={<ExecutionDetail />} />
          <Route path="schedules" element={<Schedules />} />
          <Route path="schedules/new" element={<ScheduleEditor />} />
          <Route path="schedules/:id" element={<ScheduleEditor />} />
          <Route path="channels" element={<Channels />} />
          <Route path="adhoc" element={<Adhoc />} />
          <Route path="files" element={<Files />} />
          <Route path="users" element={<Users />} />
          <Route path="audit" element={<Audit />} />
          <Route path="password" element={<Password />} />
        </Route>
      </Route>
      <Route path="*" element={<Navigate to="/" replace />} />
    </Routes>
  );
}
