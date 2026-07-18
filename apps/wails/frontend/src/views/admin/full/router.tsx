import { Navigate, Route, Routes } from "react-router-dom";

import { AdminLayout } from "./layout/AdminLayout";
import { ModelDetailPage } from "./pages/ai/ModelDetailPage";
import { ModelsListPage } from "./pages/ai/ModelsListPage";
import { VoiceDetailPage } from "./pages/ai/VoiceDetailPage";
import { VoicesListPage } from "./pages/ai/VoicesListPage";
import { WorkflowsListPage } from "./pages/ai/WorkflowsListPage";
import { WorkspacesListPage } from "./pages/ai/WorkspacesListPage";
import { FirmwareCreatePage } from "./pages/firmware/FirmwareCreatePage";
import { FirmwareDetailPage } from "./pages/firmware/FirmwareDetailPage";
import { FirmwaresListPage } from "./pages/firmware/FirmwaresListPage";
import { PeerDetailPage } from "./pages/peers/PeerDetailPage";
import { PeersListPage } from "./pages/peers/PeersListPage";
import { OverviewPage } from "./pages/overview/OverviewPage";
import { CredentialDetailPage } from "./pages/providers/CredentialDetailPage";
import { CredentialsListPage } from "./pages/providers/CredentialsListPage";
import { MiniMaxTenantDetailPage } from "./pages/providers/MiniMaxTenantDetailPage";
import { MiniMaxTenantsListPage } from "./pages/providers/MiniMaxTenantsListPage";
import {
  DashScopeTenantDetailPage,
  DashScopeTenantsListPage,
  GeminiTenantDetailPage,
  GeminiTenantsListPage,
  OpenAITenantDetailPage,
  OpenAITenantsListPage,
} from "./pages/providers/ProviderTenantPages";
import { VolcTenantDetailPage } from "./pages/providers/VolcTenantDetailPage";
import { VolcTenantsListPage } from "./pages/providers/VolcTenantsListPage";
import { ResourcesPage } from "./pages/resources/ResourcesPage";
import { ContactDetailPage } from "./pages/social/ContactDetailPage";
import { ContactsListPage } from "./pages/social/ContactsListPage";
import { FriendDetailPage } from "./pages/social/FriendDetailPage";
import { FriendGroupDetailPage } from "./pages/social/FriendGroupDetailPage";
import { FriendGroupsListPage } from "./pages/social/FriendGroupsListPage";
import { FriendsListPage } from "./pages/social/FriendsListPage";

export function AppRoutes({ contextName, onSignOut }: { contextName?: string; onSignOut(): Promise<void> }): JSX.Element {
  return (
    <Routes>
      <Route element={<AdminLayout contextName={contextName} onSignOut={onSignOut} />} path="/">
        <Route index element={<Navigate replace to="/overview" />} />
        <Route element={<OverviewPage />} path="overview" />
        <Route element={<PeersListPage />} path="peers" />
        <Route element={<PeerDetailPage />} path="peers/:publicKey" />
        <Route element={<FirmwaresListPage />} path="firmwares" />
        <Route element={<FirmwareCreatePage />} path="firmwares/new" />
        <Route element={<FirmwareDetailPage />} path="firmwares/:name" />
        <Route element={<CredentialsListPage />} path="providers/credentials" />
        <Route element={<CredentialDetailPage />} path="providers/credentials/:name" />
        <Route element={<OpenAITenantsListPage />} path="providers/openai-tenants" />
        <Route element={<OpenAITenantDetailPage />} path="providers/openai-tenants/:name" />
        <Route element={<GeminiTenantsListPage />} path="providers/gemini-tenants" />
        <Route element={<GeminiTenantDetailPage />} path="providers/gemini-tenants/:name" />
        <Route element={<DashScopeTenantsListPage />} path="providers/dashscope-tenants" />
        <Route element={<DashScopeTenantDetailPage />} path="providers/dashscope-tenants/:name" />
        <Route element={<MiniMaxTenantsListPage />} path="providers/minimax-tenants" />
        <Route element={<MiniMaxTenantDetailPage />} path="providers/minimax-tenants/:name" />
        <Route element={<VolcTenantsListPage />} path="providers/volc-tenants" />
        <Route element={<VolcTenantDetailPage />} path="providers/volc-tenants/:name" />
        <Route element={<VoicesListPage />} path="ai/voices" />
        <Route element={<VoiceDetailPage />} path="ai/voices/:id" />
        <Route element={<ModelsListPage />} path="ai/models" />
        <Route element={<ModelDetailPage />} path="ai/models/:id" />
        <Route element={<WorkflowsListPage />} path="ai/workflows" />
        <Route element={<Navigate replace to="/ai/workflows" />} path="ai/workspace-templates" />
        <Route element={<Navigate replace to="/ai/workflows" />} path="workspace-templates" />
        <Route element={<WorkspacesListPage />} path="ai/workspaces" />
        <Route element={<ResourcesPage />} path="resources" />
        <Route element={<ContactsListPage />} path="social/contacts" />
        <Route element={<ContactDetailPage />} path="social/contacts/:ownerPublicKey/:id" />
        <Route element={<FriendsListPage />} path="social/friends" />
        <Route element={<FriendDetailPage />} path="social/friends/:ownerPublicKey/:id" />
        <Route element={<FriendGroupsListPage />} path="social/friend-groups" />
        <Route element={<FriendGroupDetailPage />} path="social/friend-groups/:id" />
      </Route>
      <Route element={<Navigate replace to="/overview" />} path="*" />
    </Routes>
  );
}
