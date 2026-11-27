import { Navigate, Route, Routes } from "react-router-dom";
import AppLayout from "./components/AppLayout";
import ProtectedRoute from "./components/ProtectedRoute";
import Adhoc from "./pages/Adhoc";
import Audit from "./pages/Audit";
import Channels from "./pages/Channels";
import Credentials from "./pages/Credentials";
import ExecutionDetail from "./pages/ExecutionDetail";
import Files from "./pages/Files";
import FlowDetail from "./pages/FlowDetail";
import FlowEditor from "./pages/FlowEditor";
import FlowExecutions from "./pages/FlowExecutions";
import Flows from "./pages/Flows";
import Groups from "./pages/Groups";
import Home from "./pages/Home";
import Hosts from "./pages/Hosts";
import Login from "./pages/Login";
import Password from "./pages/Password";
import ScheduleDetail from "./pages/ScheduleDetail";
import ScheduleEditor from "./pages/ScheduleEditor";
import ScheduleExecutions from "./pages/ScheduleExecutions";
import Schedules from "./pages/Schedules";
import ToolDetail from "./pages/ToolDetail";
import ToolEditor from "./pages/ToolEditor";
import ToolExecutions from "./pages/ToolExecutions";
import ToolVersionCompare from "./pages/ToolVersionCompare";
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
          <Route path="tools/executions" element={<ToolExecutions />} />
          <Route path="tools/new" element={<ToolEditor />} />
          <Route path="tools/:id/edit" element={<ToolEditor />} />
          <Route path="tools/:id/versions/compare" element={<ToolVersionCompare />} />
          <Route path="tools/:id" element={<ToolDetail />} />
          <Route path="flows" element={<Flows />} />
          <Route path="flows/executions" element={<FlowExecutions />} />
          <Route path="flows/new" element={<FlowEditor />} />
          <Route path="flows/:id/edit" element={<FlowEditor />} />
          <Route path="flows/:id" element={<FlowDetail />} />
          <Route path="executions" element={<Navigate to="/tools/executions" replace />} />
          <Route path="executions/:id" element={<ExecutionDetail />} />
          <Route path="schedules" element={<Schedules />} />
          <Route path="schedules/executions" element={<ScheduleExecutions />} />
          <Route path="schedules/new" element={<ScheduleEditor />} />
          <Route path="schedules/:id/edit" element={<ScheduleEditor />} />
          <Route path="schedules/:id" element={<ScheduleDetail />} />
          <Route path="adhoc" element={<Adhoc />} />
          <Route path="adhoc/history" element={<Navigate to="/adhoc" replace />} />
          <Route path="channels" element={<Channels />} />
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
