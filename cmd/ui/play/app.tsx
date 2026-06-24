import { StrictMode, useCallback, useEffect, useMemo, useRef, useState } from "react";
import type { JSX, MouseEvent as ReactMouseEvent, PointerEvent as ReactPointerEvent } from "react";
import { createRoot } from "react-dom/client";
import OpenAI from "openai";
import type { ChatCompletionMessageParam } from "openai/resources/chat/completions";
import { ArrowLeft, Bot, Brain, BriefcaseBusiness, ChevronDown, Clock3, Coins, Database, Gift, KeyRound, Loader2, MessageCircle, Mic2, PawPrint, Pencil, Play, Plus, ReceiptText, RefreshCw, Search, SendHorizontal, Trash2, UserPlus, Users, Volume2, VolumeX, Workflow } from "lucide-react";
import { toast } from "sonner";
import {
  ActionBarPrimitive,
  AssistantRuntimeProvider,
  AuiIf,
  BranchPickerPrimitive,
  ComposerPrimitive,
  MessagePrimitive,
  ThreadPrimitive,
  useEditComposer,
  useLocalRuntime,
  useMessage,
  type ChatModelAdapter,
  type ChatModelRunResult,
  type EditComposerState,
  type ExportedMessageRepository,
  type ExportedMessageRepositoryItem,
  type SpeechSynthesisAdapter,
  type ThreadHistoryAdapter,
  type ThreadMessage,
} from "@assistant-ui/react";
import {
  addPeerFriend,
  addPeerFriendGroupMember,
  adoptPeerPet,
  claimPeerReward,
  clearPeerFriendGroupInviteToken,
  clearPeerFriendInviteToken,
  createPeerFriendGroup,
  createPeerFriendGroupInviteToken,
  createPeerFriendInviteToken,
  deletePeerFriend,
  deletePeerFriendGroupMember,
  deletePeerPet,
  feedPeerPet,
  getPeerFriendGroup,
  getPeerFriendGroupInviteToken,
  getPeerFriendInviteToken,
  getPeerWorkspaceHistoryAudio,
  joinPeerFriendGroup,
  getPeerReward,
  getPeerWallet,
  getPeerWalletTransaction,
  listClientVoices,
  listPeerCredentials,
  listPeerFriendGroupMembers,
  listPeerFriendGroups,
  listPeerFriends,
  listPeerModels,
  listPeerPets,
  listPeerRewards,
  listPeerVoices,
  listPeerWalletTransactions,
  listPeerWorkspaceHistory,
  listPeerWorkflows,
  listPeerWorkspaces,
  playWithPeerPet,
  putPeerFriendGroupMember,
  putPeerPet,
  streamPlayableVoices as streamPlayableVoicesSDK,
  washPeerPet,
  type FriendGroupInviteTokenGetResponse,
  type FriendGroupMemberMutableRole,
  type FriendGroupMemberObject,
  type FriendGroupObject,
  type FriendInviteTokenGetResponse,
  type FriendObject,
  type PlayVoiceStreamEvent,
} from "@gizclaw/clientservice";

import { expectData, toMessage } from "./components/api";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Empty, EmptyDescription, EmptyHeader, EmptyTitle } from "@/components/ui/empty";
import { Field as ShadField, FieldGroup, FieldLabel } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover";
import { ScrollArea } from "@/components/ui/scroll-area";
import { Select, SelectContent, SelectGroup, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Skeleton } from "@/components/ui/skeleton";
import { Sheet, SheetContent, SheetDescription, SheetHeader, SheetTitle } from "@/components/ui/sheet";
import { Switch } from "@/components/ui/switch";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { Toaster } from "@/components/ui/sonner";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { Textarea } from "@/components/ui/textarea";
import { cn } from "@/components/ui/utils";

type Section = "overview" | "friends" | "friendGroups" | "workspaces" | "workflows" | "models" | "credentials" | "voices" | "pets" | "walletTransactions" | "rewards";
type TopDrawer = "workspace" | "social-chat" | "test-chat" | null;

type ModelSpec = {
  capabilities?: ModelCapabilities;
  created_at?: string;
  description?: string;
  id: string;
  kind?: string;
  name?: string;
  owned_by?: string;
  provider?: { kind: string; name: string };
  source?: string;
  support_json_output?: boolean;
  support_temperature?: boolean;
  support_text_only?: boolean;
  support_tool_calls?: boolean;
  support_thinking?: boolean;
  thinking_param?: string;
  thinking_level_param?: string;
  thinking_levels?: string[];
  default_thinking_level?: string;
  updated_at?: string;
  use_system_role?: boolean;
};

type ModelCapabilities = {
  json_output?: boolean;
  system_role?: boolean;
  temperature?: boolean;
  text_only?: boolean;
  thinking?: {
    default_level?: string;
    level_param?: string;
    levels?: string[];
    param?: string;
    supported: boolean;
  };
  tool_calls?: boolean;
};

type Voice = {
  id: string;
  name?: string;
  provider: {
    kind: string;
    name: string;
  };
  source: string;
  updated_at?: string;
};

type ResourceItem = Record<string, unknown>;

type PageResponse<T> = {
  data?: T[];
  has_next?: boolean;
  items?: T[];
  next_cursor?: string;
};

type PagedState<T> = {
  cursors: string[];
  error: string;
  hasNext: boolean;
  items: T[];
  loading: boolean;
  nextCursor: string;
};

type WalletResource = {
  created_at: string;
  id: string;
  point_balance: number;
  token_balance: number;
  updated_at?: string;
};

type PetStats = Record<string, number>;

type PetResource = {
  ability: PetStats;
  created_at: string;
  id: string;
  life: PetStats;
  name: string;
  species_id: string;
  updated_at: string;
  voice_id: string;
};

type RewardResource = {
  badge_id: string;
  created_at: string;
  id: string;
  point_amount: number;
  prompt: string;
};

type WalletTransactionResource = {
  created_at: string;
  id: string;
  point_delta: number;
  reason: string;
  token_delta: number;
};

type ChatSession = {
  createdAt: number;
  id: string;
  title: string;
  updatedAt: number;
};

type ChatThinkingOptions = {
  enabled: boolean;
  level?: string;
};

type StoredHistory = {
  headId?: string | null;
  messages: Array<{
    message: Omit<ThreadMessage, "createdAt"> & { createdAt: string };
    parentId: string | null;
    runConfig?: ExportedMessageRepositoryItem["runConfig"];
  }>;
};

type ActiveWorkspaceState = {
  active_workspace_name?: string;
  agent_type?: string;
  message?: string;
  pending_workspace_name?: string;
  runtime_state?: string;
  workspace_mode?: WorkspaceChatMode;
  workspace_name?: string | null;
  workflow_name?: string;
};

type WorkspaceChatMode = "push" | "realtime";

type WorkspaceDetails = {
  display_name?: string;
  name: string;
  parameters?: Record<string, unknown>;
  workflow_name: string;
};

type WorkspaceHistoryEntry = {
  created_at?: string;
  gear_id?: string;
  id: string;
  name: string;
  replay_available?: boolean;
  text?: string;
  type: "agent" | "gear";
};

type SocialChatTarget = {
  id: string;
  kind: "friend" | "group";
  title: string;
  workspaceName: string;
};

type WorkspaceMemoryStats = {
  available?: boolean;
  backend?: string;
  embedding_status?: string;
  enabled?: boolean;
  index_status?: string;
  item_count?: number;
  last_updated_at?: string;
  storage_bytes?: number;
  updated_at?: string;
};

type WorkspaceRecallHit = {
  id: string;
  metadata?: Record<string, unknown>;
  score?: number;
  snippet?: string;
  created_at?: string;
  source_id?: string;
  timestamp?: string;
};

type WorkspaceRecallResponse = {
  hits?: WorkspaceRecallHit[];
};

type WebRTCSessionDescription = {
  sdp: string;
  type: RTCSdpType;
};

type PeerStreamEvent = {
  error?: string;
  kind?: "text" | "audio" | "video" | "mixed";
  label?: string;
  last_updated_at?: string;
  mime_type?: string;
  seq?: number;
  stream_id?: string;
  text?: string;
  timestamp?: number;
  type: "bos" | "eos" | "text.delta" | "text.done" | "workspace.history.updated";
  v: number;
};

type WorkspaceVoiceSession = {
  close: (reason?: string) => void;
  finishInputTurn: (error?: string) => Promise<void>;
  startInputTurn: (streamID: string) => Promise<void>;
};

type WorkspaceChatTurnStatus = "recording" | "sending" | "responding" | "playing" | "complete" | "error";

type WorkspaceChatTurn = {
  assistantText?: string;
  audioState?: "waiting" | "playing" | "done";
  createdAt: number;
  error?: string;
  id: string;
  status: WorkspaceChatTurnStatus;
  streamID?: string;
  transcript?: string;
};

const sections: Array<{ icon: typeof Bot; id: Section; label: string }> = [
  { icon: Database, id: "overview", label: "Overview" },
  { icon: UserPlus, id: "friends", label: "Friends" },
  { icon: Users, id: "friendGroups", label: "Groups" },
  { icon: BriefcaseBusiness, id: "workspaces", label: "Workspaces" },
  { icon: Workflow, id: "workflows", label: "Workflows" },
  { icon: Bot, id: "models", label: "Models" },
  { icon: KeyRound, id: "credentials", label: "Credentials" },
  { icon: Mic2, id: "voices", label: "Voices" },
  { icon: PawPrint, id: "pets", label: "Pets" },
  { icon: ReceiptText, id: "walletTransactions", label: "Transactions" },
  { icon: Gift, id: "rewards", label: "Rewards" },
];

const chatSessionsKey = "gizclaw.openai.chat.sessions";
const workspaceAudioPlaybackRequestEvent = "gizclaw:workspace-audio-play-request";
const openAIAPIKey = "gizclaw-play";
const topDrawerContentClassName =
  "top-32 h-[calc(100dvh-8rem)] w-[min(100vw,1120px)] gap-0 p-0 sm:top-24 sm:h-[calc(100dvh-6rem)] sm:max-w-none lg:top-20 lg:h-[calc(100dvh-5rem)]";

let openAIClient: OpenAI | null = null;

function getOpenAIClient(): OpenAI {
  openAIClient ??= new OpenAI({
    apiKey: openAIAPIKey,
    baseURL: `${window.location.origin}/v1`,
    dangerouslyAllowBrowser: true,
    maxRetries: 1,
  });
  return openAIClient;
}

function App(): JSX.Element {
  const [section, setSection] = useState<Section>("overview");
  const [topDrawer, setTopDrawer] = useState<TopDrawer>(null);
  const [models, setModels] = useState<ModelSpec[]>([]);
  const [wallet, setWallet] = useState<WalletResource | null>(null);
  const [selectedFriend, setSelectedFriend] = useState<FriendObject | null>(null);
  const [selectedGroup, setSelectedGroup] = useState<FriendGroupObject | null>(null);
  const [socialChatTarget, setSocialChatTarget] = useState<SocialChatTarget | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");

  const refresh = async (): Promise<void> => {
    setLoading(true);
    setError("");
    const failures: string[] = [];
    await Promise.all([
      listModels().then(setModels).catch((err: unknown) => failures.push(`models: ${toMessage(err)}`)),
      getWallet().then(setWallet).catch(() => setWallet(null)),
    ]);
    if (failures.length > 0) {
      setError(failures.join("\n"));
    }
    setLoading(false);
  };

  const openSocialChat = (target: SocialChatTarget): void => {
    setSocialChatTarget(target);
    setTopDrawer("social-chat");
  };

  useEffect(() => {
    void refresh();
  }, []);

  const counts = useMemo(
    () => ({
      models: models.length,
      overview: wallet == null ? 0 : 1,
    }),
    [models.length, wallet],
  );

  return (
    <>
      <div className="min-h-screen bg-slate-50">
      <div className="flex min-h-screen">
        <aside className="hidden w-64 shrink-0 border-r bg-background px-4 py-5 lg:block">
          <div className="mb-6 flex items-center gap-3 px-2">
            <div className="flex size-9 items-center justify-center rounded-md bg-primary text-primary-foreground">
              <Database className="size-5" />
            </div>
            <div>
              <div className="text-sm font-semibold">OpenAI Gateway</div>
              <div className="text-xs text-muted-foreground">GizClaw runtime</div>
            </div>
          </div>
          <nav className="space-y-1">
            {sections.map((item) => (
              <button
                className={cn(
                  "flex h-9 w-full items-center justify-between rounded-md px-3 text-left text-sm text-muted-foreground hover:bg-accent hover:text-accent-foreground",
                  section === item.id && "bg-accent text-accent-foreground",
                )}
                key={item.id}
                onClick={() => setSection(item.id)}
                type="button"
              >
                <span className="inline-flex items-center gap-2">
                  <item.icon className="size-4" />
                  {item.label}
                </span>
                {counts[item.id as keyof typeof counts] == null ? null : <Badge variant="outline">{counts[item.id as keyof typeof counts]}</Badge>}
              </button>
            ))}
          </nav>
        </aside>

        <main className="min-w-0 flex-1">
          <header className="border-b bg-background px-4 py-4 sm:px-6">
            <div className="flex flex-col gap-3 lg:flex-row lg:items-center lg:justify-between">
              <div>
                <div className="text-xs font-semibold uppercase text-muted-foreground">Gateway</div>
                <h1 className="text-2xl font-semibold tracking-tight">{sectionTitle(section)}</h1>
              </div>
              <div className="flex flex-wrap gap-2">
                <div className="flex gap-1 rounded-md border bg-background p-1 lg:hidden">
                  {sections.map((item) => (
                    <button
                      aria-label={item.label}
                      className={cn("flex size-8 items-center justify-center rounded-sm text-muted-foreground", section === item.id && "bg-accent text-accent-foreground")}
                      key={item.id}
                      onClick={() => setSection(item.id)}
                      type="button"
                    >
                      <item.icon className="size-4" />
                    </button>
                  ))}
                </div>
                <Button disabled={loading} onClick={() => void refresh()} size="sm" type="button" variant="outline">
                  <RefreshCw className={cn("size-4", loading && "animate-spin")} />
                  Refresh
                </Button>
                <SocialChatDrawer
                  initialTarget={socialChatTarget}
                  open={topDrawer === "social-chat"}
                  onInitialTargetChange={setSocialChatTarget}
                  onOpenChange={(nextOpen) => setTopDrawer(nextOpen ? "social-chat" : null)}
                />
                <WorkspaceDrawer open={topDrawer === "workspace"} onOpenChange={(nextOpen) => setTopDrawer(nextOpen ? "workspace" : null)} />
                <ChatTester models={models} open={topDrawer === "test-chat"} onOpenChange={(nextOpen) => setTopDrawer(nextOpen ? "test-chat" : null)} />
              </div>
            </div>
          </header>

          <div className="space-y-5 p-4 sm:p-6">
            {error !== "" ? (
              <Alert variant="destructive">
                <AlertDescription>{error}</AlertDescription>
              </Alert>
            ) : null}
            {loading ? (
              <LoadingGrid />
            ) : (
              <>
                {section === "overview" ? <OverviewPanel modelCount={models.length} wallet={wallet} /> : null}
                {section === "friends" ? (
                  selectedFriend == null ? (
                    <FriendsPanel onOpenChat={openSocialChat} onOpenFriend={setSelectedFriend} />
                  ) : (
                    <FriendDetailPanel friend={selectedFriend} onBack={() => setSelectedFriend(null)} onOpenChat={openSocialChat} />
                  )
                ) : null}
                {section === "friendGroups" ? (
                  selectedGroup == null ? (
                    <FriendGroupsPanel onOpenChat={openSocialChat} onOpenGroup={setSelectedGroup} />
                  ) : (
                    <FriendGroupDetailPanel group={selectedGroup} onBack={() => setSelectedGroup(null)} onGroupChange={setSelectedGroup} onOpenChat={openSocialChat} />
                  )
                ) : null}
                {section === "workspaces" ? <WorkspacesPanel /> : null}
                {section === "workflows" ? <WorkflowsPanel /> : null}
                {section === "models" ? <ModelsPanel initialModels={models} /> : null}
                {section === "credentials" ? <CredentialsPanel /> : null}
                {section === "voices" ? <VoicesPanel /> : null}
                {section === "pets" ? <PetsPanel /> : null}
                {section === "walletTransactions" ? <WalletTransactionsPanel /> : null}
                {section === "rewards" ? <RewardsPanel /> : null}
              </>
            )}
          </div>
        </main>
      </div>
      </div>
      <Toaster richColors />
    </>
  );
}

function FriendsPanel({ onOpenChat, onOpenFriend }: { onOpenChat: (target: SocialChatTarget) => void; onOpenFriend: (friend: FriendObject) => void }): JSX.Element {
  const pager = usePagedList<FriendObject>(listFriendsPage);
  return (
    <Tabs className="max-w-6xl" defaultValue="friends">
      <TabsList>
        <TabsTrigger value="friends">Friends</TabsTrigger>
        <TabsTrigger value="invite-token">Invite Token</TabsTrigger>
        <TabsTrigger value="add-friend">Add Friend</TabsTrigger>
      </TabsList>
      <TabsContent className="mt-4" value="friends">
        <Card>
          <CardHeader className="flex flex-row items-center justify-between gap-3">
            <CardTitle>Friends</CardTitle>
            <PageAction canNext={pager.page.hasNext} canPrevious={pager.page.cursors.length > 1} loading={pager.page.loading} onNext={pager.next} onPrevious={pager.previous} onRefresh={pager.refresh} pageIndex={pager.page.cursors.length} />
          </CardHeader>
          <CardContent>
            {pager.error !== "" ? (
              <Alert className="mb-4" variant="destructive">
                <AlertDescription>{pager.error}</AlertDescription>
              </Alert>
            ) : null}
            {pager.page.items.length === 0 ? (
              <EmptyMessage description={pager.page.loading ? "Loading friends." : "No direct friends are visible for this peer."} title={pager.page.loading ? "Loading" : "No friends"} />
            ) : (
              <div className="rounded-md border">
                <Table className="table-fixed">
                  <TableHeader>
                    <TableRow>
                      <TableHead className="w-44">Friend</TableHead>
                      <TableHead>Peer public key</TableHead>
                      <TableHead className="w-56">Workspace</TableHead>
                      <TableHead className="w-40">Updated</TableHead>
                      <TableHead className="w-44 text-right">Actions</TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {pager.page.items.map((friend) => {
                      const workspaceName = friend.workspace_name ?? "";
                      return (
                        <TableRow key={friend.id ?? friend.peer_public_key ?? workspaceName}>
                          <TableCell>
                            <div className="font-medium">{friendDisplayName(friend)}</div>
                            <div className="font-mono text-xs text-muted-foreground">{friend.id ?? "-"}</div>
                          </TableCell>
                          <TableCell className="truncate font-mono text-xs" title={friend.peer_public_key ?? ""}>{friend.peer_public_key ?? "-"}</TableCell>
                          <TableCell className="truncate" title={workspaceName}>{workspaceName || "-"}</TableCell>
                          <TableCell className="text-muted-foreground">{formatDate(friend.updated_at ?? friend.created_at)}</TableCell>
                          <TableCell>
                            <div className="flex justify-end gap-2">
                              <Button onClick={() => onOpenFriend(friend)} size="sm" type="button" variant="outline">
                                Open
                              </Button>
                              <Button disabled={workspaceName === ""} onClick={() => onOpenChat(friendChatTarget(friend))} size="sm" type="button">
                                <MessageCircle data-icon="inline-start" />
                                Chat
                              </Button>
                            </div>
                          </TableCell>
                        </TableRow>
                      );
                    })}
                  </TableBody>
                </Table>
              </div>
            )}
          </CardContent>
        </Card>
      </TabsContent>
      <TabsContent className="mt-4" value="invite-token">
        <FriendInviteTokenPanel />
      </TabsContent>
      <TabsContent className="mt-4" value="add-friend">
        <AddFriendPanel onAdded={pager.refresh} onOpenFriend={onOpenFriend} />
      </TabsContent>
    </Tabs>
  );
}

function FriendInviteTokenPanel(): JSX.Element {
  const [token, setToken] = useState<FriendInviteTokenGetResponse | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");

  const load = useCallback(async () => {
    setLoading(true);
    setError("");
    try {
      setToken(await getFriendInviteToken());
    } catch (err) {
      setError(toMessage(err));
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    void load();
  }, [load]);

  const create = async (): Promise<void> => {
    setLoading(true);
    setError("");
    try {
      setToken(await createFriendInviteToken());
      toast.success("Friend invite token refreshed");
    } catch (err) {
      setError(toMessage(err));
    } finally {
      setLoading(false);
    }
  };

  const clear = async (): Promise<void> => {
    setLoading(true);
    setError("");
    try {
      await clearFriendInviteToken();
      setToken({});
      toast.success("Friend invite token cleared");
    } catch (err) {
      setError(toMessage(err));
    } finally {
      setLoading(false);
    }
  };

  const activeToken = token?.invite_token ?? "";
  return (
    <Card>
      <CardHeader className="flex flex-row items-center justify-between gap-3">
        <CardTitle>Invite Token</CardTitle>
        <Button disabled={loading} onClick={() => void load()} size="sm" type="button" variant="outline">
          <RefreshCw className={cn("size-4", loading && "animate-spin")} />
        </Button>
      </CardHeader>
      <CardContent className="flex flex-col gap-4">
        {error !== "" ? (
          <Alert variant="destructive">
            <AlertDescription>{error}</AlertDescription>
          </Alert>
        ) : null}
        <FieldGroup>
          <ShadField>
            <FieldLabel htmlFor="friend-invite-token">Invite token</FieldLabel>
            <Input id="friend-invite-token" readOnly value={activeToken} />
          </ShadField>
          <ShadField>
            <FieldLabel htmlFor="friend-invite-token-expires">Expires</FieldLabel>
            <Input id="friend-invite-token-expires" readOnly value={formatDate(token?.expires_at)} />
          </ShadField>
        </FieldGroup>
        <div className="flex justify-end gap-2">
          {activeToken === "" ? (
            <Button disabled={loading} onClick={() => void create()} type="button">
              <RefreshCw data-icon="inline-start" />
              Refresh
            </Button>
          ) : (
            <>
              <Button disabled={loading} onClick={() => void clear()} type="button" variant="outline">
                Clear
              </Button>
              <Button disabled={loading} onClick={() => void create()} type="button">
                <RefreshCw data-icon="inline-start" />
                Refresh
              </Button>
            </>
          )}
        </div>
      </CardContent>
    </Card>
  );
}

function AddFriendPanel({ onAdded, onOpenFriend }: { onAdded: () => void; onOpenFriend: (friend: FriendObject) => void }): JSX.Element {
  const [inviteToken, setInviteToken] = useState("");
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState("");

  const submit = async (): Promise<void> => {
    const token = inviteToken.trim();
    if (token === "") {
      return;
    }
    setSaving(true);
    setError("");
    try {
      const friend = await addFriendByInviteToken(token);
      setInviteToken("");
      toast.success("Friend added", { description: friendDisplayName(friend) });
      onAdded();
      onOpenFriend(friend);
    } catch (err) {
      setError(toMessage(err));
    } finally {
      setSaving(false);
    }
  };

  return (
    <Card>
      <CardHeader>
        <CardTitle>Add Friend</CardTitle>
      </CardHeader>
      <CardContent className="flex flex-col gap-4">
        {error !== "" ? (
          <Alert variant="destructive">
            <AlertDescription>{error}</AlertDescription>
          </Alert>
        ) : null}
        <FieldGroup>
          <ShadField data-invalid={error !== ""}>
            <FieldLabel htmlFor="friend-add-token">Invite token</FieldLabel>
            <Input aria-invalid={error !== ""} id="friend-add-token" onChange={(event) => setInviteToken(event.target.value)} value={inviteToken} />
          </ShadField>
        </FieldGroup>
        <div className="flex justify-end">
          <Button disabled={saving || inviteToken.trim() === ""} onClick={() => void submit()} type="button">
            <UserPlus data-icon="inline-start" />
            Add Friend
          </Button>
        </div>
      </CardContent>
    </Card>
  );
}

function FriendDetailPanel({ friend, onBack, onOpenChat }: { friend: FriendObject; onBack: () => void; onOpenChat: (target: SocialChatTarget) => void }): JSX.Element {
  const [deleting, setDeleting] = useState(false);
  const workspaceName = friend.workspace_name ?? "";
  const history = useWorkspaceHistory(workspaceName, "desc");

  const remove = async (): Promise<void> => {
    const id = friend.id ?? "";
    if (id === "") {
      return;
    }
    setDeleting(true);
    try {
      await deleteFriend(id);
      toast.success("Friend deleted");
      onBack();
    } catch (err) {
      toast.error("Friend delete failed", { description: toMessage(err) });
    } finally {
      setDeleting(false);
    }
  };

  return (
    <div className="flex max-w-6xl flex-col gap-4">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <Button onClick={onBack} type="button" variant="outline">
          <ArrowLeft data-icon="inline-start" />
          Friends
        </Button>
        <div className="flex gap-2">
          <Button disabled={workspaceName === ""} onClick={() => onOpenChat(friendChatTarget(friend))} type="button">
            <MessageCircle data-icon="inline-start" />
            Chat
          </Button>
          <Button disabled={deleting || friend.id == null || friend.id === ""} onClick={() => void remove()} type="button" variant="destructive">
            <Trash2 data-icon="inline-start" />
            Delete
          </Button>
        </div>
      </div>
      <div className="grid gap-4 lg:grid-cols-2">
        <Card>
          <CardHeader>
            <CardTitle>Info</CardTitle>
          </CardHeader>
          <CardContent className="grid gap-x-6 gap-y-3 text-sm">
            <WorkspaceInfoItem label="Friend ID" value={friend.id ?? "-"} />
            <WorkspaceInfoItem label="Peer public key" value={friend.peer_public_key ?? "-"} />
            <WorkspaceInfoItem label="Created" value={formatDate(friend.created_at)} />
            <WorkspaceInfoItem label="Updated" value={formatDate(friend.updated_at)} />
          </CardContent>
        </Card>
        <Card>
          <CardHeader>
            <CardTitle>Workspace</CardTitle>
          </CardHeader>
          <CardContent className="grid gap-x-6 gap-y-3 text-sm">
            <WorkspaceInfoItem label="Workspace" value={workspaceName || "-"} />
            <WorkspaceInfoItem label="Conversation" value={friendDisplayName(friend)} />
          </CardContent>
        </Card>
      </div>
      <Card>
        <CardHeader className="flex flex-row items-center justify-between gap-3">
          <CardTitle>History</CardTitle>
          <Button disabled={history.loading || workspaceName === ""} onClick={history.refresh} size="sm" type="button" variant="outline">
            <RefreshCw className={cn("size-4", history.loading && "animate-spin")} />
          </Button>
        </CardHeader>
        <CardContent className="p-0">
          <WorkspaceHistoryPanel error={history.error} history={history.items} loading={history.loading} onPlay={(entry) => playWorkspaceHistoryAsset(workspaceName, entry.id)} />
        </CardContent>
      </Card>
    </div>
  );
}

function FriendGroupsPanel({ onOpenChat, onOpenGroup }: { onOpenChat: (target: SocialChatTarget) => void; onOpenGroup: (group: FriendGroupObject) => void }): JSX.Element {
  const pager = usePagedList<FriendGroupObject>(listFriendGroupsPage);
  const [createOpen, setCreateOpen] = useState(false);
  const [createName, setCreateName] = useState("");
  const [createDescription, setCreateDescription] = useState("");
  const [creating, setCreating] = useState(false);
  const [joinOpen, setJoinOpen] = useState(false);
  const [joinToken, setJoinToken] = useState("");
  const [joining, setJoining] = useState(false);

  const create = async (): Promise<void> => {
    const name = createName.trim();
    const description = createDescription.trim();
    if (name === "") {
      return;
    }
    setCreating(true);
    try {
      const group = await createFriendGroup(name, description);
      setCreateName("");
      setCreateDescription("");
      setCreateOpen(false);
      toast.success("Group created", { description: groupDisplayName(group) });
      pager.refresh();
      onOpenGroup(group);
    } catch (err) {
      toast.error("Group create failed", { description: toMessage(err) });
    } finally {
      setCreating(false);
    }
  };

  const join = async (): Promise<void> => {
    const token = joinToken.trim();
    if (token === "") {
      return;
    }
    setJoining(true);
    try {
      const response = await joinFriendGroupByInviteToken(token);
      setJoinToken("");
      setJoinOpen(false);
      toast.success("Group joined", { description: groupDisplayName(response.group) });
      pager.refresh();
      onOpenGroup(response.group);
    } catch (err) {
      toast.error("Group join failed", { description: toMessage(err) });
    } finally {
      setJoining(false);
    }
  };

  return (
    <div className="max-w-6xl">
      {pager.error !== "" ? (
        <Alert className="mb-4" variant="destructive">
          <AlertDescription>{pager.error}</AlertDescription>
        </Alert>
      ) : null}
      <Card>
        <CardHeader className="flex flex-row items-center justify-between gap-3">
          <CardTitle>Groups</CardTitle>
          <div className="flex gap-2">
            <Button onClick={() => setCreateOpen(true)} size="sm" type="button">
              <Plus data-icon="inline-start" />
              Create Group
            </Button>
            <Button onClick={() => setJoinOpen(true)} size="sm" type="button">
              <Users data-icon="inline-start" />
              Join Group
            </Button>
            <PageAction canNext={pager.page.hasNext} canPrevious={pager.page.cursors.length > 1} loading={pager.page.loading} onNext={pager.next} onPrevious={pager.previous} onRefresh={pager.refresh} pageIndex={pager.page.cursors.length} />
          </div>
        </CardHeader>
        <CardContent>
          {pager.page.items.length === 0 ? (
            <EmptyMessage description={pager.page.loading ? "Loading groups." : "No friend groups are visible for this peer."} title={pager.page.loading ? "Loading" : "No groups"} />
          ) : (
            <div className="rounded-md border">
              <Table className="table-fixed">
                <TableHeader>
                  <TableRow>
                    <TableHead className="w-56">Group</TableHead>
                    <TableHead className="w-28">My role</TableHead>
                    <TableHead>Workspace</TableHead>
                    <TableHead className="w-40">Updated</TableHead>
                    <TableHead className="w-44 text-right">Actions</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {pager.page.items.map((group) => {
                    const workspaceName = group.workspace_name ?? "";
                    return (
                      <TableRow key={group.id ?? group.name ?? workspaceName}>
                        <TableCell>
                          <div className="font-medium">{groupDisplayName(group)}</div>
                          <div className="font-mono text-xs text-muted-foreground">{group.id ?? "-"}</div>
                        </TableCell>
                        <TableCell>{group.my_role ?? "-"}</TableCell>
                        <TableCell className="truncate" title={workspaceName}>{workspaceName || "-"}</TableCell>
                        <TableCell className="text-muted-foreground">{formatDate(group.updated_at ?? group.created_at)}</TableCell>
                        <TableCell>
                          <div className="flex justify-end gap-2">
                            <Button onClick={() => onOpenGroup(group)} size="sm" type="button" variant="outline">
                              Open
                            </Button>
                            <Button disabled={workspaceName === ""} onClick={() => onOpenChat(groupChatTarget(group))} size="sm" type="button">
                              <MessageCircle data-icon="inline-start" />
                              Chat
                            </Button>
                          </div>
                        </TableCell>
                      </TableRow>
                    );
                  })}
                </TableBody>
              </Table>
            </div>
          )}
        </CardContent>
      </Card>
      <Dialog open={createOpen} onOpenChange={setCreateOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Create Group</DialogTitle>
            <DialogDescription>Create a group workspace owned by this peer.</DialogDescription>
          </DialogHeader>
          <FieldGroup>
            <Field label="Name" value={createName} onChange={setCreateName} />
            <TextAreaField label="Description" value={createDescription} onChange={setCreateDescription} />
          </FieldGroup>
          <DialogFooter>
            <Button disabled={creating} onClick={() => setCreateOpen(false)} type="button" variant="outline">
              Cancel
            </Button>
            <Button disabled={creating || createName.trim() === ""} onClick={() => void create()} type="button">
              Create Group
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
      <Dialog open={joinOpen} onOpenChange={setJoinOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Join Group</DialogTitle>
            <DialogDescription>Join a group by its invite token.</DialogDescription>
          </DialogHeader>
          <FieldGroup>
            <ShadField>
              <FieldLabel htmlFor="group-join-token">Invite token</FieldLabel>
              <Input id="group-join-token" onChange={(event) => setJoinToken(event.target.value)} value={joinToken} />
            </ShadField>
          </FieldGroup>
          <DialogFooter>
            <Button disabled={joining} onClick={() => setJoinOpen(false)} type="button" variant="outline">
              Cancel
            </Button>
            <Button disabled={joining || joinToken.trim() === ""} onClick={() => void join()} type="button">
              Join Group
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}

function FriendGroupDetailPanel({
  group,
  onBack,
  onGroupChange,
  onOpenChat,
}: {
  group: FriendGroupObject;
  onBack: () => void;
  onGroupChange: (group: FriendGroupObject) => void;
  onOpenChat: (target: SocialChatTarget) => void;
}): JSX.Element {
  const [currentGroup, setCurrentGroup] = useState(group);
  const groupID = currentGroup.id ?? "";
  const workspaceName = currentGroup.workspace_name ?? "";
  const history = useWorkspaceHistory(workspaceName, "desc");

  useEffect(() => {
    setCurrentGroup(group);
  }, [group]);

  const refreshGroup = async (): Promise<void> => {
    if (groupID === "") {
      return;
    }
    try {
      const next = await getFriendGroup(groupID);
      setCurrentGroup(next);
      onGroupChange(next);
    } catch (err) {
      toast.error("Group refresh failed", { description: toMessage(err) });
    }
  };

  return (
    <div className="flex max-w-6xl flex-col gap-4">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <Button onClick={onBack} type="button" variant="outline">
          <ArrowLeft data-icon="inline-start" />
          Groups
        </Button>
        <Button disabled={workspaceName === ""} onClick={() => onOpenChat(groupChatTarget(currentGroup))} type="button">
          <MessageCircle data-icon="inline-start" />
          Chat
        </Button>
      </div>
      <Tabs defaultValue="info">
        <TabsList>
          <TabsTrigger value="info">Info</TabsTrigger>
          <TabsTrigger value="members">Members</TabsTrigger>
          <TabsTrigger value="invite-token">Invite Token</TabsTrigger>
          <TabsTrigger value="history">History</TabsTrigger>
        </TabsList>
        <TabsContent className="mt-4" value="info">
          <GroupInfoPanel group={currentGroup} onRefresh={refreshGroup} />
        </TabsContent>
        <TabsContent className="mt-4" value="members">
          <GroupMembersPanel group={currentGroup} />
        </TabsContent>
        <TabsContent className="mt-4" value="invite-token">
          <GroupInviteTokenPanel group={currentGroup} />
        </TabsContent>
        <TabsContent className="mt-4" value="history">
          <Card>
            <CardHeader className="flex flex-row items-center justify-between gap-3">
              <CardTitle>History</CardTitle>
              <Button disabled={history.loading || workspaceName === ""} onClick={history.refresh} size="sm" type="button" variant="outline">
                <RefreshCw className={cn("size-4", history.loading && "animate-spin")} />
              </Button>
            </CardHeader>
            <CardContent className="p-0">
              <WorkspaceHistoryPanel error={history.error} history={history.items} loading={history.loading} onPlay={(entry) => playWorkspaceHistoryAsset(workspaceName, entry.id)} />
            </CardContent>
          </Card>
        </TabsContent>
      </Tabs>
    </div>
  );
}

function GroupInfoPanel({ group, onRefresh }: { group: FriendGroupObject; onRefresh: () => Promise<void> }): JSX.Element {
  return (
    <Card>
      <CardHeader className="flex flex-row items-center justify-between gap-3">
        <CardTitle>Info</CardTitle>
        <Button onClick={() => void onRefresh()} size="sm" type="button" variant="outline">
          <RefreshCw data-icon="inline-start" />
          Refresh
        </Button>
      </CardHeader>
      <CardContent className="grid gap-x-6 gap-y-3 text-sm sm:grid-cols-2">
        <WorkspaceInfoItem label="Group ID" value={group.id ?? "-"} />
        <WorkspaceInfoItem label="Name" value={group.name ?? "-"} />
        <WorkspaceInfoItem label="My role" value={group.my_role ?? "-"} />
        <WorkspaceInfoItem label="Workspace" value={group.workspace_name ?? "-"} />
        <WorkspaceInfoItem label="Created by" value={group.created_by_peer_public_key ?? "-"} />
        <WorkspaceInfoItem label="Updated" value={formatDate(group.updated_at ?? group.created_at)} />
      </CardContent>
    </Card>
  );
}

function GroupMembersPanel({ group }: { group: FriendGroupObject }): JSX.Element {
  const groupID = group.id ?? "";
  const pager = usePagedList<FriendGroupMemberObject>(useCallback((cursor: string) => listFriendGroupMembersPage(groupID, cursor), [groupID]));
  const [memberPublicKey, setMemberPublicKey] = useState("");
  const [memberRole, setMemberRole] = useState<FriendGroupMemberMutableRole>("member");
  const [saving, setSaving] = useState(false);
  const canManage = group.my_role === "owner" || group.my_role === "admin";

  const addMember = async (): Promise<void> => {
    const peerPublicKey = memberPublicKey.trim();
    if (groupID === "" || peerPublicKey === "") {
      return;
    }
    setSaving(true);
    try {
      await addFriendGroupMember(groupID, peerPublicKey, memberRole);
      setMemberPublicKey("");
      toast.success("Group member added");
      pager.refresh();
    } catch (err) {
      toast.error("Group member add failed", { description: toMessage(err) });
    } finally {
      setSaving(false);
    }
  };

  return (
    <Card>
      <CardHeader className="flex flex-row items-center justify-between gap-3">
        <CardTitle>Members</CardTitle>
        <PageAction canNext={pager.page.hasNext} canPrevious={pager.page.cursors.length > 1} loading={pager.page.loading} onNext={pager.next} onPrevious={pager.previous} onRefresh={pager.refresh} pageIndex={pager.page.cursors.length} />
      </CardHeader>
      <CardContent className="flex flex-col gap-4">
        {pager.error !== "" ? (
          <Alert variant="destructive">
            <AlertDescription>{pager.error}</AlertDescription>
          </Alert>
        ) : null}
        {canManage ? (
          <div className="grid gap-3 rounded-md border p-3 md:grid-cols-[minmax(0,1fr)_160px_auto]">
            <Field label="Peer public key" value={memberPublicKey} onChange={setMemberPublicKey} />
            <SelectField label="Role" value={memberRole} onChange={(value) => setMemberRole(value === "admin" ? "admin" : "member")} options={["member", "admin"]} />
            <div className="flex items-end">
              <Button disabled={saving || memberPublicKey.trim() === ""} onClick={() => void addMember()} type="button">
                <Plus data-icon="inline-start" />
                Add
              </Button>
            </div>
          </div>
        ) : null}
        {pager.page.items.length === 0 ? (
          <EmptyMessage description={pager.page.loading ? "Loading members." : "No group members are visible."} title={pager.page.loading ? "Loading" : "No members"} />
        ) : (
          <div className="rounded-md border">
            <Table className="table-fixed">
              <TableHeader>
                <TableRow>
                  <TableHead>Peer public key</TableHead>
                  <TableHead className="w-28">Role</TableHead>
                  <TableHead className="w-40">Updated</TableHead>
                  <TableHead className="w-56 text-right">Actions</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {pager.page.items.map((member) => (
                  <GroupMemberRow canManage={canManage} groupID={groupID} key={member.id ?? member.peer_public_key ?? ""} member={member} onChanged={pager.refresh} />
                ))}
              </TableBody>
            </Table>
          </div>
        )}
      </CardContent>
    </Card>
  );
}

function GroupMemberRow({ canManage, groupID, member, onChanged }: { canManage: boolean; groupID: string; member: FriendGroupMemberObject; onChanged: () => void }): JSX.Element {
  const [saving, setSaving] = useState(false);
  const memberID = member.id ?? "";
  const mutable = canManage && member.role !== "owner" && groupID !== "" && memberID !== "";
  const updateRole = async (role: FriendGroupMemberMutableRole): Promise<void> => {
    setSaving(true);
    try {
      await updateFriendGroupMember(groupID, memberID, role);
      toast.success("Group member updated");
      onChanged();
    } catch (err) {
      toast.error("Group member update failed", { description: toMessage(err) });
    } finally {
      setSaving(false);
    }
  };
  const remove = async (): Promise<void> => {
    setSaving(true);
    try {
      await deleteFriendGroupMember(groupID, memberID);
      toast.success("Group member removed");
      onChanged();
    } catch (err) {
      toast.error("Group member remove failed", { description: toMessage(err) });
    } finally {
      setSaving(false);
    }
  };

  return (
    <TableRow>
      <TableCell className="truncate font-mono text-xs" title={member.peer_public_key ?? ""}>{member.peer_public_key ?? "-"}</TableCell>
      <TableCell>{member.role ?? "-"}</TableCell>
      <TableCell className="text-muted-foreground">{formatDate(member.updated_at ?? member.created_at)}</TableCell>
      <TableCell>
        {mutable ? (
          <div className="flex justify-end gap-2">
            <Button disabled={saving || member.role === "admin"} onClick={() => void updateRole("admin")} size="sm" type="button" variant="outline">
              Admin
            </Button>
            <Button disabled={saving || member.role === "member"} onClick={() => void updateRole("member")} size="sm" type="button" variant="outline">
              Member
            </Button>
            <Button disabled={saving} onClick={() => void remove()} size="sm" type="button" variant="destructive">
              Remove
            </Button>
          </div>
        ) : (
          <div className="text-right text-xs text-muted-foreground">-</div>
        )}
      </TableCell>
    </TableRow>
  );
}

function GroupInviteTokenPanel({ group }: { group: FriendGroupObject }): JSX.Element {
  const groupID = group.id ?? "";
  const owner = group.my_role === "owner";
  const [token, setToken] = useState<FriendGroupInviteTokenGetResponse | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");

  const load = useCallback(async () => {
    if (groupID === "" || !owner) {
      setToken(null);
      return;
    }
    setLoading(true);
    setError("");
    try {
      setToken(await getFriendGroupInviteToken(groupID));
    } catch (err) {
      setError(toMessage(err));
    } finally {
      setLoading(false);
    }
  }, [groupID, owner]);

  useEffect(() => {
    void load();
  }, [load]);

  const create = async (): Promise<void> => {
    setLoading(true);
    setError("");
    try {
      setToken(await createFriendGroupInviteToken(groupID));
      toast.success("Group invite token refreshed");
    } catch (err) {
      setError(toMessage(err));
    } finally {
      setLoading(false);
    }
  };

  const clear = async (): Promise<void> => {
    setLoading(true);
    setError("");
    try {
      await clearFriendGroupInviteToken(groupID);
      setToken({});
      toast.success("Group invite token cleared");
    } catch (err) {
      setError(toMessage(err));
    } finally {
      setLoading(false);
    }
  };

  if (!owner) {
    return <EmptyMessage description="Only the group owner can manage the invite token." title="Invite token unavailable" />;
  }

  const activeToken = token?.invite_token ?? "";
  return (
    <Card>
      <CardHeader className="flex flex-row items-center justify-between gap-3">
        <CardTitle>Invite Token</CardTitle>
        <Button disabled={loading} onClick={() => void load()} size="sm" type="button" variant="outline">
          <RefreshCw className={cn("size-4", loading && "animate-spin")} />
        </Button>
      </CardHeader>
      <CardContent className="flex flex-col gap-4">
        {error !== "" ? (
          <Alert variant="destructive">
            <AlertDescription>{error}</AlertDescription>
          </Alert>
        ) : null}
        <FieldGroup>
          <ShadField>
            <FieldLabel htmlFor="group-invite-token">Invite token</FieldLabel>
            <Input id="group-invite-token" readOnly value={activeToken} />
          </ShadField>
          <ShadField>
            <FieldLabel htmlFor="group-invite-token-expires">Expires</FieldLabel>
            <Input id="group-invite-token-expires" readOnly value={formatDate(token?.expires_at)} />
          </ShadField>
        </FieldGroup>
        <div className="flex justify-end gap-2">
          {activeToken !== "" ? (
            <Button disabled={loading} onClick={() => void clear()} type="button" variant="outline">
              Clear
            </Button>
          ) : null}
          <Button disabled={loading} onClick={() => void create()} type="button">
            <RefreshCw data-icon="inline-start" />
            Refresh
          </Button>
        </div>
      </CardContent>
    </Card>
  );
}

function SocialChatDrawer({
  initialTarget,
  onInitialTargetChange,
  onOpenChange,
  open,
}: {
  initialTarget: SocialChatTarget | null;
  onInitialTargetChange: (target: SocialChatTarget | null) => void;
  onOpenChange: (open: boolean) => void;
  open: boolean;
}): JSX.Element {
  const [friends, setFriends] = useState<FriendObject[]>([]);
  const [groups, setGroups] = useState<FriendGroupObject[]>([]);
  const [targetKey, setTargetKey] = useState("");
  const [state, setState] = useState<ActiveWorkspaceState | null>(null);
  const [mode, setMode] = useState<WorkspaceChatMode>("push");
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");
  const targets = useMemo(() => [...friends.map(friendChatTarget), ...groups.map(groupChatTarget)].filter((target) => target.workspaceName !== ""), [friends, groups]);
  const selectedTarget = useMemo(() => targets.find((target) => socialTargetKey(target) === targetKey) ?? initialTarget ?? targets[0] ?? null, [initialTarget, targetKey, targets]);
  const history = useWorkspaceHistory(selectedTarget?.workspaceName ?? "", "asc");

  const loadTargets = useCallback(async () => {
    setLoading(true);
    setError("");
    try {
      const [friendPage, groupPage] = await Promise.all([listFriendsPage(""), listFriendGroupsPage("")]);
      const nextFriends = friendPage.items ?? friendPage.data ?? [];
      const nextGroups = groupPage.items ?? groupPage.data ?? [];
      setFriends(nextFriends);
      setGroups(nextGroups);
      const allTargets = [...nextFriends.map(friendChatTarget), ...nextGroups.map(groupChatTarget)].filter((target) => target.workspaceName !== "");
      const nextTarget = initialTarget != null && allTargets.some((target) => socialTargetKey(target) === socialTargetKey(initialTarget)) ? initialTarget : allTargets[0] ?? null;
      setTargetKey(nextTarget == null ? "" : socialTargetKey(nextTarget));
    } catch (err) {
      setError(toMessage(err));
    } finally {
      setLoading(false);
    }
  }, [initialTarget]);

  useEffect(() => {
    if (open) {
      void loadTargets();
    }
  }, [loadTargets, open]);

  useEffect(() => {
    if (!open || selectedTarget == null) {
      return;
    }
    onInitialTargetChange(selectedTarget);
    setError("");
    setLoading(true);
    void setActiveWorkspace(selectedTarget.workspaceName)
      .then((nextState) => {
        const normalized = normalizeWorkspaceState(nextState);
        setState(normalized);
        setMode(normalized.workspace_mode ?? "push");
      })
      .catch((err: unknown) => setError(toMessage(err)))
      .finally(() => setLoading(false));
  }, [onInitialTargetChange, open, selectedTarget?.workspaceName, selectedTarget]);

  const updateWorkspaceMode = async (nextMode: WorkspaceChatMode): Promise<void> => {
    const workspaceName = selectedTarget?.workspaceName ?? "";
    if (workspaceName === "") {
      return;
    }
    setError("");
    setLoading(true);
    try {
      const nextState = normalizeWorkspaceState(await setActiveWorkspaceMode(nextMode, workspaceName));
      setState(nextState);
      setMode(nextState.workspace_mode ?? nextMode);
      toast.success("Chat mode updated", { description: nextMode === "push" ? "Push To Talk" : "Realtime Chat" });
    } catch (err) {
      setError(toMessage(err));
    } finally {
      setLoading(false);
    }
  };

  const playHistory = async (entry: WorkspaceHistoryEntry): Promise<void> => {
    try {
      requestWorkspaceAudioPlayback();
      await playActiveWorkspaceHistory(entry.id);
      toast.success("History replay started", { description: entry.id });
    } catch (err) {
      toast.error("History replay failed", { description: workspaceFeatureMessage(err) });
    }
  };

  return (
    <Sheet modal={false} open={open} onOpenChange={onOpenChange}>
      <Button aria-pressed={open} onClick={() => onOpenChange(!open)} size="sm" type="button" variant={open ? "default" : "outline"}>
        <MessageCircle data-icon="inline-start" />
        Chat
      </Button>
      <SheetContent
        className={topDrawerContentClassName}
        onInteractOutside={(event) => event.preventDefault()}
        overlayClassName="pointer-events-none top-32 bg-transparent sm:top-24 lg:top-20"
        side="right"
      >
        <SheetHeader className="border-b px-5 py-4 pr-12">
          <SheetTitle>Chat</SheetTitle>
          <SheetDescription>Append voice messages and replay history through the selected social workspace.</SheetDescription>
          <div className="grid gap-3 pt-2 md:grid-cols-[220px_minmax(0,1fr)_auto]">
            <Select value={selectedTarget == null ? "" : socialTargetKey(selectedTarget)} onValueChange={setTargetKey}>
              <SelectTrigger>
                <SelectValue placeholder="Conversation" />
              </SelectTrigger>
              <SelectContent>
                <SelectGroup>
                  {targets.map((target) => (
                    <SelectItem key={socialTargetKey(target)} value={socialTargetKey(target)}>
                      {target.kind === "friend" ? "Friend" : "Group"} / {target.title}
                    </SelectItem>
                  ))}
                </SelectGroup>
              </SelectContent>
            </Select>
            <div className="min-w-0 rounded-md border px-3 py-2 text-sm">
              <div className="truncate font-medium">{selectedTarget?.title ?? "No conversation"}</div>
              <div className="truncate text-xs text-muted-foreground">{selectedTarget?.workspaceName ?? "-"}</div>
            </div>
            <Button disabled={loading} onClick={() => void loadTargets()} type="button" variant="outline">
              <RefreshCw className={cn("size-4", loading && "animate-spin")} />
            </Button>
          </div>
        </SheetHeader>
        <div className="flex min-h-0 flex-1 flex-col gap-4 p-5">
          {error !== "" ? (
            <Alert variant="destructive">
              <AlertDescription>{error}</AlertDescription>
            </Alert>
          ) : null}
          {selectedTarget == null ? (
            <EmptyMessage description="No friend or group conversation has a workspace yet." title="No chat target" />
          ) : (
            <>
              <WorkspaceChatPanel mode={mode} onHistoryChange={history.loadNewer} onModeChange={(nextMode) => void updateWorkspaceMode(nextMode)} showTurns={false} state={state} title="Composer" />
              <Card className="min-h-0 flex-1">
                <CardHeader className="flex flex-row items-center justify-between gap-3">
                  <CardTitle className="flex min-w-0 items-center gap-2 text-sm">
                    <span className="truncate">History</span>
                    {history.lastUpdatedAt !== "" ? <Badge variant="outline">{formatDate(history.lastUpdatedAt)}</Badge> : null}
                  </CardTitle>
                  <Button disabled={history.loading} onClick={history.refresh} size="sm" type="button" variant="outline">
                    <RefreshCw className={cn("size-4", history.loading && "animate-spin")} />
                  </Button>
                </CardHeader>
                <CardContent className="min-h-0 p-0">
                  <ChatHistoryTimeline error={history.error} history={history.items} loading={history.loading} onPlay={playHistory} />
                </CardContent>
              </Card>
            </>
          )}
        </div>
      </SheetContent>
    </Sheet>
  );
}

function WorkspaceDrawer({ onOpenChange, open }: { onOpenChange: (open: boolean) => void; open: boolean }): JSX.Element {
  const [workspaces, setWorkspaces] = useState<ResourceItem[]>([]);
  const [state, setState] = useState<ActiveWorkspaceState | null>(null);
  const [selectedWorkspace, setSelectedWorkspace] = useState("");
  const [history, setHistory] = useState<WorkspaceHistoryEntry[]>([]);
  const [historyError, setHistoryError] = useState("");
  const [memory, setMemory] = useState<WorkspaceMemoryStats | null>(null);
  const [memoryError, setMemoryError] = useState("");
  const [workspaceDetails, setWorkspaceDetails] = useState<WorkspaceDetails | null>(null);
  const [workspaceDetailsError, setWorkspaceDetailsError] = useState("");
  const [workspaceParametersText, setWorkspaceParametersText] = useState("{}");
  const [workspaceSaving, setWorkspaceSaving] = useState(false);
  const [recallQuery, setRecallQuery] = useState("");
  const [recallHits, setRecallHits] = useState<WorkspaceRecallHit[]>([]);
  const [recallError, setRecallError] = useState("");
  const [mode, setMode] = useState<WorkspaceChatMode>("push");
  const [workspaceTab, setWorkspaceTab] = useState("chat");
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");

  const activeWorkspace = state?.active_workspace_name ?? "";
  const pendingWorkspace = state?.pending_workspace_name ?? "";
  const currentWorkspace = state?.workspace_name ?? "";
  const runtimeState = state?.runtime_state ?? (currentWorkspace === "" ? "no active workspace" : "unknown");
  const pendingDirty = selectedWorkspace !== "" && selectedWorkspace !== currentWorkspace;
  const canSetWorkspace = pendingDirty && !loading;
  const canReloadWorkspace = currentWorkspace !== "" && !pendingDirty && !loading;

  const loadWorkspaceDetails = useCallback(async (workspaceName: string) => {
    const name = workspaceName.trim();
    if (name === "") {
      setWorkspaceDetails(null);
      setWorkspaceParametersText("{}");
      setWorkspaceDetailsError("");
      return;
    }
    try {
      const details = await getWorkspaceDetails(name);
      setWorkspaceDetails(details);
      setWorkspaceParametersText(formatWorkspaceParameters(details.parameters));
      setWorkspaceDetailsError("");
    } catch (err) {
      setWorkspaceDetails(null);
      setWorkspaceDetailsError(workspaceFeatureMessage(err));
    }
  }, []);

  const loadWorkspaceIntrospection = useCallback(async (workspaceName: string) => {
    setHistoryError("");
    setMemoryError("");
    setRecallError("");
    setRecallHits([]);
    if (workspaceName === "") {
      setHistory([]);
      setMemory(null);
      return;
    }

    const [nextHistory, nextMemory] = await Promise.allSettled([getActiveWorkspaceHistory(), getActiveWorkspaceMemoryStats()]);
    if (nextHistory.status === "fulfilled") {
      setHistory(nextHistory.value.items ?? nextHistory.value.data ?? []);
    } else {
      setHistory([]);
      setHistoryError(workspaceFeatureMessage(nextHistory.reason));
    }
    if (nextMemory.status === "fulfilled") {
      setMemory(nextMemory.value);
    } else {
      setMemory(null);
      setMemoryError(workspaceFeatureMessage(nextMemory.reason));
    }
  }, []);

  const loadDrawer = useCallback(async () => {
    setLoading(true);
    setError("");
    try {
      const [workspacePage, workspaceState] = await Promise.all([listPeerResourcePage("workspaces", ""), getActiveWorkspace()]);
      const nextState = normalizeWorkspaceState(workspaceState);
      setWorkspaces(sortWorkspacesByActivity(workspacePage.items ?? workspacePage.data ?? []));
      setState(nextState);
      setSelectedWorkspace(nextState.workspace_name ?? "");
      setMode(nextState.workspace_mode ?? "push");
      await loadWorkspaceDetails(nextState.workspace_name ?? "");
      await loadWorkspaceIntrospection(nextState.active_workspace_name ?? nextState.workspace_name ?? "");
    } catch (err) {
      setError(toMessage(err));
    } finally {
      setLoading(false);
    }
  }, [loadWorkspaceDetails, loadWorkspaceIntrospection]);

  useEffect(() => {
    if (open) {
      void loadDrawer();
    }
  }, [loadDrawer, open]);

  const setWorkspaceSelection = async (): Promise<void> => {
    const workspaceName = selectedWorkspace.trim();
    if (workspaceName === "") {
      return;
    }
    setError("");
    setLoading(true);
    try {
      const nextState = normalizeWorkspaceState(await setActiveWorkspace(workspaceName));
      setState(nextState);
      setSelectedWorkspace(nextState.workspace_name ?? workspaceName);
      setMode(nextState.workspace_mode ?? "push");
      await loadWorkspaceDetails(nextState.workspace_name ?? workspaceName);
      await loadWorkspaceIntrospection(nextState.active_workspace_name ?? "");
      toast.success("Workspace selection updated", { description: workspaceName });
    } catch (err) {
      setError(toMessage(err));
    } finally {
      setLoading(false);
    }
  };

  const reloadWorkspace = async (): Promise<void> => {
    setError("");
    setLoading(true);
    try {
      const nextState = normalizeWorkspaceState(await reloadActiveWorkspace());
      setState(nextState);
      setSelectedWorkspace(nextState.workspace_name ?? "");
      setMode(nextState.workspace_mode ?? "push");
      await loadWorkspaceDetails(nextState.workspace_name ?? "");
      await loadWorkspaceIntrospection(nextState.active_workspace_name ?? nextState.workspace_name ?? "");
      toast.success("Workspace runtime reloaded", { description: nextState.active_workspace_name ?? nextState.workspace_name ?? "" });
    } catch (err) {
      setError(toMessage(err));
    } finally {
      setLoading(false);
    }
  };

  const playHistory = async (entry: WorkspaceHistoryEntry): Promise<void> => {
    setHistoryError("");
    try {
      requestWorkspaceAudioPlayback();
      await playActiveWorkspaceHistory(entry.id);
      setWorkspaceTab("chat");
      toast.success("History replay started", { description: entry.id });
    } catch (err) {
      setHistoryError(workspaceFeatureMessage(err));
    }
  };

  const refreshActiveWorkspaceIntrospection = useCallback(() => {
    const workspaceName = state?.active_workspace_name ?? state?.workspace_name ?? selectedWorkspace;
    if (workspaceName === "") {
      return;
    }
    void loadWorkspaceIntrospection(workspaceName);
  }, [loadWorkspaceIntrospection, selectedWorkspace, state?.active_workspace_name, state?.workspace_name]);

  const runRecall = async (): Promise<void> => {
    const query = recallQuery.trim();
    if (query === "") {
      return;
    }
    setRecallError("");
    setLoading(true);
    try {
      const response = await recallActiveWorkspace(query);
      setRecallHits(response.hits ?? []);
    } catch (err) {
      setRecallHits([]);
      setRecallError(workspaceFeatureMessage(err));
    } finally {
      setLoading(false);
    }
  };

  const updateWorkspaceMode = async (nextMode: WorkspaceChatMode): Promise<void> => {
    setError("");
    setLoading(true);
    try {
      const nextState = normalizeWorkspaceState(await setActiveWorkspaceMode(nextMode, currentWorkspace || selectedWorkspace));
      setState(nextState);
      setSelectedWorkspace(nextState.workspace_name ?? "");
      setMode(nextState.workspace_mode ?? nextMode);
      await loadWorkspaceDetails(nextState.workspace_name ?? "");
      await loadWorkspaceIntrospection(nextState.active_workspace_name ?? nextState.workspace_name ?? "");
      toast.success("Workspace mode reloaded", { description: nextMode === "push" ? "Push To Talk" : "Realtime Chat" });
    } catch (err) {
      setError(toMessage(err));
    } finally {
      setLoading(false);
    }
  };

  const saveWorkspaceDetails = async (): Promise<void> => {
    const workspaceName = (workspaceDetails?.name ?? currentWorkspace ?? selectedWorkspace).trim();
    if (workspaceName === "") {
      setWorkspaceDetailsError("Select a workspace before saving.");
      return;
    }
    let parameters: Record<string, unknown>;
    try {
      parameters = parseWorkspaceParameters(workspaceParametersText);
    } catch (err) {
      setWorkspaceDetailsError(toMessage(err));
      return;
    }
    setWorkspaceSaving(true);
    try {
      const updated = await updateWorkspaceDetails({
        parameters,
        workspace_name: workspaceName,
        workflow_name: workspaceDetails?.workflow_name ?? "",
      });
      setWorkspaceDetails(updated);
      setWorkspaceParametersText(formatWorkspaceParameters(updated.parameters));
      setWorkspaceDetailsError("");
      toast.success("Workspace saved", { description: "Reload the workspace runtime to apply changes." });
      await loadDrawer();
    } catch (err) {
      setWorkspaceDetailsError(workspaceFeatureMessage(err));
    } finally {
      setWorkspaceSaving(false);
    }
  };

  return (
    <Sheet modal={false} open={open} onOpenChange={onOpenChange}>
      <Button aria-pressed={open} onClick={() => onOpenChange(!open)} size="sm" type="button" variant={open ? "default" : "outline"}>
        <BriefcaseBusiness data-icon="inline-start" />
        Workspace
      </Button>
      <SheetContent
        className={topDrawerContentClassName}
        onInteractOutside={(event) => event.preventDefault()}
        overlayClassName="pointer-events-none top-32 bg-transparent sm:top-24 lg:top-20"
        side="right"
      >
        <SheetHeader className="border-b px-5 py-4 pr-12">
          <div className="flex flex-col gap-2">
            <SheetTitle>Workspace</SheetTitle>
            <SheetDescription>Inspect and test the current peer run active workspace.</SheetDescription>
          </div>
          <div className="grid items-end gap-3 pt-2 sm:grid-cols-[minmax(0,1fr)_auto]">
            <div className="min-w-0">
              <ScrollableSelectField
                label="Active workspace"
                loading={loading}
                value={selectedWorkspace}
                onChange={setSelectedWorkspace}
                options={workspaces.map((workspace) => stringField(workspace, "name")).filter((name) => name !== "")}
              />
            </div>
            <div className="flex items-end gap-2">
              <Button className="shrink-0" disabled={!canSetWorkspace} onClick={() => void setWorkspaceSelection()} type="button" variant="outline">
                Set
              </Button>
              <Button className="shrink-0" disabled={!canReloadWorkspace} onClick={() => void reloadWorkspace()} type="button">
                <RefreshCw data-icon="inline-start" />
                Reload
              </Button>
            </div>
          </div>
          <Card className="mt-1">
            <CardHeader className="pb-2">
              <CardTitle className="flex items-center justify-between gap-3 text-sm">
                <span>Runtime</span>
                <Badge variant={runtimeState === "active" ? "default" : "outline"}>{runtimeState}</Badge>
              </CardTitle>
            </CardHeader>
            <CardContent className="grid gap-x-6 gap-y-3 text-sm sm:grid-cols-2 lg:grid-cols-3">
              <WorkspaceInfoItem label="Display name" value={workspaceDetails?.display_name || "-"} />
              <WorkspaceInfoItem label="Workspace ID" value={currentWorkspace || selectedWorkspace || "-"} />
              <WorkspaceInfoItem label="Selected" value={selectedWorkspace || "-"} />
              <WorkspaceInfoItem label="Pending" value={pendingWorkspace || "-"} />
              <WorkspaceInfoItem label="Active" value={activeWorkspace || "-"} />
              <WorkspaceInfoItem label="Workflow" value={state?.workflow_name || "-"} />
              <WorkspaceInfoItem label="Agent" value={state?.agent_type || "unavailable"} />
            </CardContent>
          </Card>
        </SheetHeader>
        <div className="flex min-h-0 flex-1 flex-col">
          {error !== "" ? (
            <Alert className="m-4 mb-0" variant="destructive">
              <AlertDescription>{error}</AlertDescription>
            </Alert>
          ) : null}
          <Tabs className="flex min-h-0 flex-1 flex-col" value={workspaceTab} onValueChange={setWorkspaceTab}>
            <div className="border-b px-5 py-3">
              <TabsList>
                <TabsTrigger value="chat">Chat</TabsTrigger>
                <TabsTrigger value="history">History</TabsTrigger>
                <TabsTrigger value="memory">Memory</TabsTrigger>
                <TabsTrigger value="recall">Recall</TabsTrigger>
                <TabsTrigger value="settings">Settings</TabsTrigger>
              </TabsList>
            </div>
            <TabsContent forceMount className={cn("m-0 min-h-0 flex-1", workspaceTab !== "chat" && "hidden")} value="chat">
              <WorkspaceChatPanel mode={mode} onModeChange={(nextMode) => {
                void updateWorkspaceMode(nextMode);
              }} onHistoryChange={refreshActiveWorkspaceIntrospection} state={state} />
            </TabsContent>
            <TabsContent forceMount className={cn("m-0 min-h-0 flex-1", workspaceTab !== "history" && "hidden")} value="history">
              <WorkspaceHistoryPanel error={historyError} history={history} loading={loading} onPlay={playHistory} />
            </TabsContent>
            <TabsContent forceMount className={cn("m-0 min-h-0 flex-1", workspaceTab !== "memory" && "hidden")} value="memory">
              <WorkspaceMemoryPanel error={memoryError} memory={memory} />
            </TabsContent>
            <TabsContent forceMount className={cn("m-0 min-h-0 flex-1", workspaceTab !== "recall" && "hidden")} value="recall">
              <WorkspaceRecallPanel error={recallError} hits={recallHits} loading={loading} query={recallQuery} onQueryChange={setRecallQuery} onRun={runRecall} />
            </TabsContent>
            <TabsContent forceMount className={cn("m-0 min-h-0 flex-1", workspaceTab !== "settings" && "hidden")} value="settings">
              <WorkspaceDetailsPanel
                details={workspaceDetails}
                error={workspaceDetailsError}
                loading={loading}
                parametersText={workspaceParametersText}
                saving={workspaceSaving}
                onParametersChange={setWorkspaceParametersText}
                onRefresh={() => void loadWorkspaceDetails(currentWorkspace || selectedWorkspace)}
                onSave={() => void saveWorkspaceDetails()}
              />
            </TabsContent>
          </Tabs>
        </div>
      </SheetContent>
    </Sheet>
  );
}

function WorkspaceInfoItem({ label, value }: { label: string; value: string }): JSX.Element {
  return (
    <div className="min-w-0">
      <div className="text-xs font-medium text-muted-foreground">{label}</div>
      <div className="truncate text-foreground">{value}</div>
    </div>
  );
}

function WorkspaceDetailsPanel({
  details,
  error,
  loading,
  onParametersChange,
  onRefresh,
  onSave,
  parametersText,
  saving,
}: {
  details: WorkspaceDetails | null;
  error: string;
  loading: boolean;
  onParametersChange: (value: string) => void;
  onRefresh: () => void;
  onSave: () => void;
  parametersText: string;
  saving: boolean;
}): JSX.Element {
  const disabled = details == null || loading || saving;
  return (
    <ScrollArea className="h-full">
      <div className="flex flex-col gap-4 p-5">
        {error !== "" ? (
          <Alert variant="destructive">
            <AlertDescription>{error}</AlertDescription>
          </Alert>
        ) : null}
        <Card>
          <CardHeader className="pb-3">
            <CardTitle className="flex items-center justify-between gap-3 text-sm">
              <span>Workspace Info</span>
              {details != null ? <Badge variant="outline">{details.display_name || details.name}</Badge> : null}
            </CardTitle>
          </CardHeader>
          <CardContent>
            {details == null ? (
              <EmptyMessage description="Select a workspace to edit its configuration." title="No workspace selected" />
            ) : (
              <div className="flex flex-col gap-5">
                <div className="grid gap-x-6 gap-y-3 text-sm sm:grid-cols-2">
                  <WorkspaceInfoItem label="Display name" value={details.display_name || "-"} />
                  <WorkspaceInfoItem label="Workspace ID" value={details.name || "-"} />
                  <WorkspaceInfoItem label="Workflow" value={details.workflow_name || "-"} />
                </div>
                <FieldGroup>
                <ShadField data-invalid={error !== ""}>
                  <FieldLabel htmlFor="workspace-parameters">Parameters</FieldLabel>
                  <Textarea
                    aria-invalid={error !== ""}
                    className="min-h-64 font-mono text-sm"
                    disabled={disabled}
                    id="workspace-parameters"
                    spellCheck={false}
                    value={parametersText}
                    onChange={(event) => onParametersChange(event.target.value)}
                  />
                </ShadField>
                <div className="flex items-center justify-end gap-2">
                  <Button disabled={loading || saving} onClick={onRefresh} type="button" variant="outline">
                    <RefreshCw data-icon="inline-start" />
                    Refresh
                  </Button>
                  <Button disabled={disabled} onClick={onSave} type="button">
                    <Pencil data-icon="inline-start" />
                    Save
                  </Button>
                </div>
                </FieldGroup>
              </div>
            )}
          </CardContent>
        </Card>
      </div>
    </ScrollArea>
  );
}

function workspaceTurnStatusLabel(status: WorkspaceChatTurnStatus): string {
  switch (status) {
    case "recording":
      return "recording";
    case "sending":
      return "sending";
    case "responding":
      return "responding";
    case "playing":
      return "playing";
    case "complete":
      return "complete";
    case "error":
      return "error";
  }
}

function workspaceTurnBadgeVariant(status: WorkspaceChatTurnStatus): "default" | "secondary" | "outline" | "destructive" {
  switch (status) {
    case "recording":
    case "playing":
      return "default";
    case "error":
      return "destructive";
    case "complete":
      return "outline";
    default:
      return "secondary";
  }
}

function splitWorkspaceStreamID(streamID?: string): { prefix: string; suffix: string } {
  const normalized = streamID?.trim() ?? "";
  if (normalized === "") {
    return { prefix: "", suffix: "" };
  }
  const index = normalized.indexOf(":");
  if (index < 0) {
    return { prefix: normalized, suffix: "" };
  }
  return {
    prefix: normalized.slice(0, index),
    suffix: normalized.slice(index + 1),
  };
}

function WorkspaceChatPanel({
  mode,
  onHistoryChange,
  onModeChange,
  showTurns = true,
  state,
  title = "Conversation",
}: {
  mode: WorkspaceChatMode;
  onHistoryChange?: (lastUpdatedAt?: string) => void;
  onModeChange: (value: WorkspaceChatMode) => void;
  showTurns?: boolean;
  state: ActiveWorkspaceState | null;
  title?: string;
}): JSX.Element {
  const activeWorkspaceName = state?.active_workspace_name ?? "";
  const hasActiveWorkspace = activeWorkspaceName !== "";
  const [status, setStatus] = useState<"idle" | "connecting" | "connected" | "error">("idle");
  const [inputActive, setInputActive] = useState(false);
  const [inputPressed, setInputPressed] = useState(false);
  const [error, setError] = useState("");
  const [turns, setTurns] = useState<WorkspaceChatTurn[]>([]);
  const [modeMenuOpen, setModeMenuOpen] = useState(false);
  const audioRef = useRef<HTMLAudioElement | null>(null);
  const sessionRef = useRef<WorkspaceVoiceSession | null>(null);
  const currentTurnIDRef = useRef<string | null>(null);
  const inputActiveRef = useRef(false);
  const inputPressedRef = useRef(false);
  const inputStartingRef = useRef(false);
  const inputFinishPendingRef = useRef<string | undefined>(undefined);
  const streamTextRef = useRef<Map<string, string>>(new Map());
  const streamTurnRef = useRef<Map<string, string>>(new Map());

  const closeRecordingTurnsExcept = useCallback((activeTurnID?: string) => {
    setTurns((current) =>
      current.map((turn) => {
        if (turn.status !== "recording" || turn.id === activeTurnID) {
          return turn;
        }
        return { ...turn, status: "sending" };
      }),
    );
  }, []);

  const createTurn = useCallback(
    (status: WorkspaceChatTurnStatus, streamID?: string) => {
      const turn: WorkspaceChatTurn = {
        audioState: "waiting",
        createdAt: Date.now(),
        id: `${Date.now()}-${Math.random().toString(16).slice(2)}`,
        status,
        streamID: streamID == null || streamID === "" ? undefined : streamID,
      };
      currentTurnIDRef.current = turn.id;
      if (streamID != null && streamID !== "") {
        streamTurnRef.current.set(streamID, turn.id);
      }
      setTurns((current) => [
        ...current
          .map((existing) => {
            if (existing.status !== "recording") {
              return existing;
            }
            return { ...existing, status: "sending" as WorkspaceChatTurnStatus };
          })
          .slice(-19),
        turn,
      ]);
      return turn.id;
    },
    [],
  );

  const updateTurn = useCallback((targetID: string, patch: Partial<WorkspaceChatTurn>) => {
    setTurns((current) => current.map((turn) => (turn.id === targetID ? { ...turn, ...patch } : turn)));
  }, []);

  const turnIDForStream = useCallback(
    (streamID: string | undefined, status: WorkspaceChatTurnStatus) => {
      const normalized = streamID ?? "";
      if (normalized !== "") {
        const existing = streamTurnRef.current.get(normalized);
        if (existing != null) {
          currentTurnIDRef.current = existing;
          closeRecordingTurnsExcept(existing);
          return existing;
        }
        return createTurn(status, normalized);
      }
      let id = currentTurnIDRef.current;
      if (id == null) {
        id = createTurn(status);
      }
      return id;
    },
    [closeRecordingTurnsExcept, createTurn],
  );

  const updateCurrentTurn = useCallback((patch: Partial<WorkspaceChatTurn>) => {
    let id = currentTurnIDRef.current;
    if (id == null) {
      id = createTurn("responding");
    }
    updateTurn(id, patch);
  }, [createTurn, updateTurn]);

  const playWorkspaceAudio = useCallback(() => {
    void unlockBrowserAudio();
    if (audioRef.current != null) {
      void audioRef.current.play().catch(() => undefined);
    }
  }, []);

  const notifyHistoryChange = useCallback((lastUpdatedAt?: string) => {
    if (onHistoryChange == null) {
      return;
    }
    window.setTimeout(() => onHistoryChange(lastUpdatedAt), 1000);
  }, [onHistoryChange]);

  const handlePeerEvent = useCallback(
    (event: PeerStreamEvent) => {
      const updateEventTurn = (patch: Partial<WorkspaceChatTurn>, status: WorkspaceChatTurnStatus = "responding"): string => {
        const targetID = turnIDForStream(event.stream_id, status);
        updateTurn(targetID, patch);
        return targetID;
      };
      if (event.type === "workspace.history.updated") {
        notifyHistoryChange(event.last_updated_at);
        return;
      }
      if ((event.type === "text.delta" || event.type === "text.done") && event.text != null) {
        const label = (event.label ?? "").toLowerCase();
        const key = `${event.stream_id ?? "default"}:${label}`;
        const current = streamTextRef.current.get(key) ?? "";
        const next = event.type === "text.done" ? event.text || current : current + event.text;
        streamTextRef.current.set(key, next);
        if (label.includes("transcript")) {
          updateEventTurn({ status: "responding", transcript: next });
        } else {
          updateEventTurn({ assistantText: next, status: "responding" });
        }
      }
      const eventError = event.error?.trim() ?? "";
      if (eventError !== "") {
        if (eventError === "interrupted") {
          updateEventTurn({ status: "complete" }, "responding");
          return;
        }
        updateEventTurn({ error: eventError, status: "error" }, "error");
        return;
      }
      if (event.type === "eos" && event.text == null) {
        if (event.kind === "audio") {
          const targetID = updateEventTurn({ audioState: "done", status: "complete" }, "responding");
          if (currentTurnIDRef.current === targetID) {
            currentTurnIDRef.current = null;
          }
          notifyHistoryChange();
        }
      }
      if (event.type === "bos" && event.kind === "audio") {
        playWorkspaceAudio();
        updateEventTurn({ audioState: "playing", status: "playing" }, "playing");
      }
    },
    [notifyHistoryChange, playWorkspaceAudio, turnIDForStream, updateTurn],
  );

  const closeSession = useCallback((reason?: string) => {
    const session = sessionRef.current;
    sessionRef.current = null;
    if (session != null) {
      session.close(reason);
    }
    if (audioRef.current != null) {
      audioRef.current.srcObject = null;
    }
    inputActiveRef.current = false;
    inputPressedRef.current = false;
    inputStartingRef.current = false;
    inputFinishPendingRef.current = undefined;
    streamTextRef.current.clear();
    streamTurnRef.current.clear();
    setInputActive(false);
    setInputPressed(false);
    setStatus("idle");
  }, []);

  const ensureSession = useCallback(async () => {
    if (sessionRef.current != null || status === "connecting") {
      return;
    }
    setError("");
    setStatus("connecting");
    try {
      const session = await createWorkspaceVoiceSession({
        onEvent: handlePeerEvent,
        onRemoteStream: (stream) => {
          if (audioRef.current == null) {
            return;
          }
          audioRef.current.srcObject = stream;
          playWorkspaceAudio();
        },
        onState: (stateName) => {
          if (stateName === "failed" || stateName === "disconnected" || stateName === "closed") {
            sessionRef.current = null;
            setStatus(stateName === "closed" ? "idle" : "error");
          }
        },
      });
      sessionRef.current = session;
      setStatus("connected");
    } catch (err) {
      const message = toMessage(err);
      setError(message);
      setStatus("error");
      toast.error("Workspace voice failed", { description: message });
    }
  }, [handlePeerEvent, playWorkspaceAudio, status]);

  useEffect(() => {
    if (!hasActiveWorkspace) {
      closeSession("workspace closed");
      return;
    }
    void ensureSession();
  }, [closeSession, ensureSession, hasActiveWorkspace]);

  const startInputTurn = useCallback(async () => {
    if (sessionRef.current == null || inputActiveRef.current || inputStartingRef.current) {
      return;
    }
    inputStartingRef.current = true;
    inputPressedRef.current = true;
    setInputPressed(true);
    playWorkspaceAudio();
    const streamID = newWorkspaceAudioStreamID();
    const turnID = createTurn("recording", streamID);
    try {
      await sessionRef.current.startInputTurn(streamID);
      inputActiveRef.current = true;
      setInputActive(true);
      setTurns((current) => current.map((turn) => (turn.id === turnID ? { ...turn, audioState: "waiting", status: "recording" } : turn)));
      const pendingReason = inputFinishPendingRef.current;
      if (pendingReason !== undefined) {
        inputFinishPendingRef.current = undefined;
        try {
          await sessionRef.current.finishInputTurn(pendingReason);
        } finally {
          inputActiveRef.current = false;
          inputPressedRef.current = false;
          setInputActive(false);
          setInputPressed(false);
          setTurns((current) => current.map((turn) => (turn.id === turnID ? { ...turn, status: pendingReason === "" ? "sending" : "error", error: pendingReason === "" ? turn.error : pendingReason } : turn)));
        }
      }
    } catch (err) {
      inputFinishPendingRef.current = undefined;
      inputPressedRef.current = false;
      setInputPressed(false);
      const message = toMessage(err);
      setTurns((current) => current.map((turn) => (turn.id === turnID ? { ...turn, error: message, status: "error" } : turn)));
      setError(message);
      toast.error("Workspace microphone failed", { description: message });
    } finally {
      inputStartingRef.current = false;
    }
  }, [createTurn, playWorkspaceAudio]);

  useEffect(() => {
    window.addEventListener(workspaceAudioPlaybackRequestEvent, playWorkspaceAudio);
    return () => {
      window.removeEventListener(workspaceAudioPlaybackRequestEvent, playWorkspaceAudio);
    };
  }, [playWorkspaceAudio]);

  const finishInputTurn = useCallback(
    async (reason?: string) => {
      if (inputStartingRef.current) {
        inputFinishPendingRef.current = reason ?? "";
        inputPressedRef.current = false;
        setInputPressed(false);
        if (sessionRef.current != null) {
          try {
            await sessionRef.current.finishInputTurn(reason);
          } catch {
            // The start path will surface microphone errors.
          }
        }
        return;
      }
      if (sessionRef.current == null || !inputActiveRef.current) {
        inputPressedRef.current = false;
        setInputPressed(false);
        return;
      }
      try {
        await sessionRef.current.finishInputTurn(reason);
      } finally {
        inputActiveRef.current = false;
        inputPressedRef.current = false;
        setInputActive(false);
        setInputPressed(false);
        updateCurrentTurn({ status: reason == null || reason === "" ? "sending" : "error", ...(reason == null || reason === "" ? {} : { error: reason }) });
      }
    },
    [updateCurrentTurn],
  );

  useEffect(() => () => closeSession("drawer closed"), [closeSession]);

  useEffect(() => {
    if (mode !== "push" || !inputPressed) {
      return;
    }
    const finish = (): void => {
      void finishInputTurn();
    };
    window.addEventListener("pointerup", finish);
    window.addEventListener("blur", finish);
    return () => {
      window.removeEventListener("pointerup", finish);
      window.removeEventListener("blur", finish);
    };
  }, [finishInputTurn, inputPressed, mode]);

  const connected = status === "connected";
  const buttonLabel = mode === "push" ? (inputPressed ? "Release to stop" : "Push to talk") : inputPressed ? "Stop realtime chat" : "Start realtime chat";
  const statusLabel = status === "idle" ? "stopped" : status;

  const handlePrimaryPointerDown = (event: ReactPointerEvent<HTMLButtonElement>): void => {
    if (mode !== "push" || !hasActiveWorkspace || !connected) {
      return;
    }
    if (event.pointerType === "mouse" && event.button !== 0) {
      return;
    }
    event.preventDefault();
    event.currentTarget.setPointerCapture(event.pointerId);
    void startInputTurn();
  };

  const handlePrimaryPointerUp = (event: ReactPointerEvent<HTMLButtonElement>): void => {
    if (mode !== "push") {
      return;
    }
    event.preventDefault();
    if (event.currentTarget.hasPointerCapture(event.pointerId)) {
      event.currentTarget.releasePointerCapture(event.pointerId);
    }
    void finishInputTurn();
  };

  const handlePrimaryClick = (event: ReactMouseEvent<HTMLButtonElement>): void => {
    if (mode === "push") {
      event.preventDefault();
      event.stopPropagation();
      return;
    }
    if (mode !== "realtime" || !hasActiveWorkspace || !connected) {
      return;
    }
    if (inputActive) {
      void finishInputTurn();
      return;
    }
    void startInputTurn();
  };

  return (
    <div className={cn("flex flex-col gap-4 p-5", showTurns ? "h-full" : "shrink-0 rounded-md border bg-background")}>
      <div className="flex items-start justify-between gap-3">
        <div className="min-w-0">
          <div className="text-sm font-semibold">{title}</div>
          <div className="mt-1 flex flex-wrap gap-2">
            <Badge variant={connected ? "default" : "secondary"}>{statusLabel}</Badge>
            {activeWorkspaceName !== "" ? <Badge variant="outline">{activeWorkspaceName}</Badge> : null}
          </div>
        </div>
        {hasActiveWorkspace ? (
          <div className="flex h-10 shrink-0">
            <Button
              className={cn("h-10 rounded-r-none", inputPressed && "bg-primary text-primary-foreground ring-2 ring-primary/40 ring-offset-2 ring-offset-background")}
              data-state={inputPressed ? "pressed" : "idle"}
              disabled={!hasActiveWorkspace || !connected}
              id="workspace-chat-primary-trigger"
              type="button"
              onClick={handlePrimaryClick}
              onContextMenu={(event) => {
                if (mode === "push") {
                  event.preventDefault();
                }
              }}
              onKeyDown={(event) => {
                if (mode === "push" && (event.key === " " || event.key === "Enter") && !event.repeat) {
                  event.preventDefault();
                  void startInputTurn();
                }
              }}
              onKeyUp={(event) => {
                if (mode === "push" && (event.key === " " || event.key === "Enter")) {
                  event.preventDefault();
                  void finishInputTurn();
                }
              }}
              onPointerCancel={(event) => {
                if (mode === "push" && event.currentTarget.hasPointerCapture(event.pointerId)) {
                  event.currentTarget.releasePointerCapture(event.pointerId);
                  void finishInputTurn("push to talk canceled");
                }
              }}
              onPointerDown={handlePrimaryPointerDown}
              onPointerUp={handlePrimaryPointerUp}
              style={mode === "push" ? { touchAction: "none", userSelect: "none" } : undefined}
            >
              <Mic2 data-icon="inline-start" />
              <span>{buttonLabel}</span>
            </Button>
            <Popover open={modeMenuOpen} onOpenChange={setModeMenuOpen}>
              <PopoverTrigger asChild>
                <Button
                  aria-label="Switch workspace chat mode"
                  className="h-10 w-10 shrink-0 rounded-l-none px-0"
                  disabled={inputActive || status === "connecting"}
                  id="workspace-chat-mode-trigger"
                  type="button"
                >
                  <ChevronDown data-icon="inline-end" />
                </Button>
              </PopoverTrigger>
              <PopoverContent align="end" className="w-56 p-1">
                <WorkspaceModeOption current={mode === "push"} label="Push To Talk" onSelect={() => {
                  onModeChange("push");
                  setModeMenuOpen(false);
                }} />
                <WorkspaceModeOption current={mode === "realtime"} label="Realtime Chat" onSelect={() => {
                  onModeChange("realtime");
                  setModeMenuOpen(false);
                }} />
              </PopoverContent>
            </Popover>
          </div>
        ) : null}
      </div>
      {hasActiveWorkspace ? (
        <div className="flex min-h-0 flex-1 flex-col gap-4">
          <audio ref={audioRef} autoPlay playsInline />
          {error !== "" ? (
            <Alert className="shrink-0" variant="destructive">
              <AlertDescription>{error}</AlertDescription>
            </Alert>
          ) : null}
          {showTurns ? (
            <ScrollArea className="min-h-0 flex-1 rounded-md border bg-background">
              <div className="flex flex-col gap-3 p-4 text-sm">
                {turns.length === 0 ? (
                  <EmptyMessage description="Hold the button to start a voice turn." title="No conversation turns" />
                ) : (
                  Array.from(turns).reverse().map((turn) => {
                    const streamMeta = splitWorkspaceStreamID(turn.streamID);
                    return (
                      <div className="rounded-md border bg-card px-3 py-3" key={turn.id}>
                        <div className="flex flex-wrap items-center justify-between gap-2">
                          <div className="flex flex-wrap items-center gap-2">
                            {streamMeta.prefix !== "" ? <Badge variant="outline">{streamMeta.prefix}</Badge> : null}
                            <Badge variant={workspaceTurnBadgeVariant(turn.status)}>{workspaceTurnStatusLabel(turn.status)}</Badge>
                            {turn.status === "recording" ? <Badge variant="secondary">BOS sent</Badge> : null}
                            {turn.status !== "recording" && turn.status !== "error" ? <Badge variant="secondary">EOS sent</Badge> : null}
                            {turn.audioState != null ? <Badge variant="outline">audio {turn.audioState}</Badge> : null}
                          </div>
                          <span className="text-xs text-muted-foreground">{formatDate(turn.createdAt)}</span>
                        </div>
                        {turn.transcript != null && turn.transcript !== "" ? (
                          <div className="mt-3 rounded-md bg-muted px-3 py-2">
                            <div className="flex items-center justify-between gap-2 text-xs font-medium text-muted-foreground">
                              <span>You</span>
                              {streamMeta.suffix !== "" ? <span className="font-mono">{streamMeta.suffix}</span> : null}
                            </div>
                            <div className="whitespace-pre-wrap break-words">{turn.transcript}</div>
                          </div>
                        ) : null}
                        {turn.assistantText != null && turn.assistantText !== "" ? (
                          <div className="mt-3 rounded-md bg-secondary px-3 py-2">
                            <div className="flex items-center justify-between gap-2 text-xs font-medium text-muted-foreground">
                              <span>Assistant</span>
                              {streamMeta.suffix !== "" ? <span className="font-mono">{streamMeta.suffix}</span> : null}
                            </div>
                            <div className="whitespace-pre-wrap break-words">{turn.assistantText}</div>
                          </div>
                        ) : null}
                        {turn.error != null && turn.error !== "" ? <div className="mt-3 text-sm text-destructive">{turn.error}</div> : null}
                      </div>
                    );
                  })
                )}
              </div>
            </ScrollArea>
          ) : null}
        </div>
      ) : (
        <EmptyMessage description="Select an active workspace before starting conversation tests." title="No active workspace" />
      )}
    </div>
  );
}

function WorkspaceHistoryPanel({ error, history, loading, onPlay }: { error: string; history: WorkspaceHistoryEntry[]; loading: boolean; onPlay: (entry: WorkspaceHistoryEntry) => Promise<void> }): JSX.Element {
  if (loading && history.length === 0) {
    return <LoadingGrid />;
  }
  if (error !== "") {
    return <EmptyMessage description={error} title="History unavailable" />;
  }
  if (history.length === 0) {
    return <EmptyMessage description="No history is available for the active workspace." title="No history" />;
  }
  return (
    <ScrollArea className="h-full">
      <div className="p-5">
        <div className="min-w-0 overflow-hidden rounded-md border">
          <Table className="table-fixed">
            <TableHeader>
              <TableRow>
                <TableHead className="w-36">Time</TableHead>
                <TableHead className="w-20">Type</TableHead>
                <TableHead className="w-44">Name</TableHead>
                <TableHead>Text</TableHead>
                <TableHead className="w-24 text-right">Replay</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {history.map((entry) => {
                const replayable = entry.replay_available === true;
                const entryName = entry.type === "gear" && entry.gear_id != null && entry.gear_id !== "" ? `${entry.name} / ${entry.gear_id}` : entry.name;
                return (
                  <TableRow key={entry.id}>
                    <TableCell className="truncate text-muted-foreground">{formatDate(entry.created_at)}</TableCell>
                    <TableCell className="truncate">{entry.type}</TableCell>
                    <TableCell className="truncate" title={entryName}>{entryName}</TableCell>
                    <TableCell className="truncate" title={entry.text || entry.id}>{entry.text || entry.id}</TableCell>
                    <TableCell className="text-right">
                      {replayable ? (
                        <Button onClick={() => void onPlay(entry)} size="sm" type="button" variant="outline">
                          <Play data-icon="inline-start" />
                          Play
                        </Button>
                      ) : (
                        <span className="text-xs text-muted-foreground">-</span>
                      )}
                    </TableCell>
                  </TableRow>
                );
              })}
            </TableBody>
          </Table>
        </div>
      </div>
    </ScrollArea>
  );
}

function WorkspaceModeOption({ current, label, onSelect }: { current: boolean; label: string; onSelect: () => void }): JSX.Element {
  return (
    <button
      aria-pressed={current}
      className={cn(
        "flex w-full items-center justify-between gap-3 rounded-sm px-2 py-1.5 text-left text-sm hover:bg-accent hover:text-accent-foreground",
        current && "bg-accent text-accent-foreground",
      )}
      onClick={onSelect}
      type="button"
    >
      <span>{label}</span>
      {current ? <Badge variant="outline">Current</Badge> : null}
    </button>
  );
}

function WorkspaceMemoryPanel({ error, memory }: { error: string; memory: WorkspaceMemoryStats | null }): JSX.Element {
  if (error !== "") {
    return <EmptyMessage description={error} title="Memory unavailable" />;
  }
  if (memory == null || memory.available === false) {
    return <EmptyMessage description="Memory stats are unavailable for the active workspace." title="Memory unavailable" />;
  }
  const rows = [
    ["Enabled", memory.enabled === false ? "No" : "Yes"],
    ["Items", String(memory.item_count ?? 0)],
    ["Storage", formatBytes(memory.storage_bytes)],
    ["Embedding", memory.embedding_status ?? "-"],
    ["Index", memory.index_status ?? "-"],
    ["Backend", memory.backend ?? "-"],
    ["Updated", formatDate(memory.last_updated_at ?? memory.updated_at)],
  ];
  return (
    <div className="p-5">
      <Card>
        <CardHeader>
          <CardTitle className="text-sm">Memory</CardTitle>
        </CardHeader>
        <CardContent className="grid gap-x-6 gap-y-3 text-sm sm:grid-cols-2 lg:grid-cols-3">
          {rows.map(([label, value]) => (
            <WorkspaceInfoItem key={label} label={label} value={value} />
          ))}
        </CardContent>
      </Card>
    </div>
  );
}

function WorkspaceRecallPanel({ error, hits, loading, onQueryChange, onRun, query }: { error: string; hits: WorkspaceRecallHit[]; loading: boolean; onQueryChange: (value: string) => void; onRun: () => Promise<void>; query: string }): JSX.Element {
  return (
    <div className="flex h-full flex-col gap-4 p-5">
      <div className="flex gap-2">
        <Input onChange={(event) => onQueryChange(event.target.value)} onKeyDown={(event) => {
          if (event.key === "Enter") {
            void onRun();
          }
        }} placeholder="Recall query" value={query} />
        <Button aria-busy={loading} disabled={loading || query.trim() === ""} onClick={() => void onRun()} type="button">
          {loading ? <Loader2 className="animate-spin" data-icon="inline-start" /> : <Search data-icon="inline-start" />}
          {loading ? "Running" : "Run Recall"}
        </Button>
      </div>
      {error !== "" ? (
        <EmptyMessage description={error} title="Recall unavailable" />
      ) : hits.length === 0 ? (
        <EmptyMessage description="Run a recall query to inspect active workspace matches." title="No recall results" />
      ) : (
        <ScrollArea className="min-h-0 flex-1 rounded-md border">
          <div className="flex flex-col gap-3 p-4">
            {hits.map((hit) => (
              <Card key={hit.id}>
                <CardHeader>
                  <CardTitle className="flex items-center justify-between gap-3 text-sm">
                    <span>{hit.source_id ?? hit.id}</span>
                    <Badge variant="outline">{formatScore(hit.score)}</Badge>
                  </CardTitle>
                </CardHeader>
                <CardContent className="grid gap-2 text-sm">
                  <div>{hit.snippet ?? "No snippet"}</div>
                  <div className="flex items-center gap-1 text-xs text-muted-foreground">
                    <Clock3 className="size-3" />
                    {formatDate(hit.created_at ?? hit.timestamp)}
                  </div>
                </CardContent>
              </Card>
            ))}
          </div>
        </ScrollArea>
      )}
    </div>
  );
}

function ChatTester({ models, onOpenChange, open }: { models: ModelSpec[]; onOpenChange: (open: boolean) => void; open: boolean }): JSX.Element {
  const [sessions, setSessions] = useState<ChatSession[]>(() => loadChatSessions());
  const [activeSessionID, setActiveSessionID] = useState(() => sessions[0]?.id ?? createChatSession().id);
  const [selectedModel, setSelectedModel] = useState("");
  const [selectedVoice, setSelectedVoice] = useState("");
  const [voices, setVoices] = useState<Voice[]>([]);
  const [voicesLoading, setVoicesLoading] = useState(false);
  const [voicesLoaded, setVoicesLoaded] = useState(false);
  const [autoSpeak, setAutoSpeak] = useState(false);
  const [systemPrompt, setSystemPrompt] = useState("");
  const [temperature, setTemperature] = useState("0.7");
  const [thinkingEnabled, setThinkingEnabled] = useState(true);
  const [thinkingLevel, setThinkingLevel] = useState("");
  const [chatError, setChatError] = useState("");
  const [resetToken, setResetToken] = useState(0);
  const selectedModelSpec = useMemo(() => models.find((model) => model.id === selectedModel), [models, selectedModel]);
  const playableVoices = useMemo(() => voices.filter(isPlayableVoice), [voices]);
  const thinkingLevels = useMemo(() => selectedModelSpec?.thinking_levels ?? [], [selectedModelSpec]);
  const supportsThinking = selectedModelSpec?.support_thinking === true;
  const supportsTemperature = selectedModelSpec?.support_temperature !== false;

  const reportChatError = useCallback((message: string) => {
    setChatError(message);
    if (message.trim() !== "") {
      toast.error("Chat request failed", { description: message });
    }
  }, []);

  const setAutoSpeakEnabled = useCallback((checked: boolean) => {
    setAutoSpeak(checked);
  }, []);

  const loadVoices = useCallback(() => {
    if (voicesLoading || voicesLoaded) {
      return;
    }
    setVoicesLoading(true);
    void streamPlayableVoices((voice) => {
      setVoices((current) => mergeVoices([...current, voice]));
    })
      .then(() => setVoicesLoaded(true))
      .catch((err: unknown) => {
        reportChatError(`Voices request failed: ${toMessage(err)}`);
      })
      .finally(() => setVoicesLoading(false));
  }, [reportChatError, voicesLoaded, voicesLoading]);

  useEffect(() => {
    if (sessions.length === 0) {
      const session = createChatSession();
      setSessions([session]);
      setActiveSessionID(session.id);
      return;
    }
    saveChatSessions(sessions);
  }, [sessions]);

  useEffect(() => {
    if (selectedModel === "" && models.length > 0) {
      setSelectedModel(models[0].id);
    }
  }, [models, selectedModel]);

  useEffect(() => {
    if (open) {
      loadVoices();
    }
  }, [loadVoices, open]);

  useEffect(() => {
    if (playableVoices.length === 0) {
      setSelectedVoice("");
      return;
    }
    if (!playableVoices.some((voice) => voice.id === selectedVoice)) {
      setSelectedVoice(playableVoices[0].id);
    }
  }, [playableVoices, selectedVoice]);

  useEffect(() => {
    if (!supportsThinking) {
      setThinkingLevel("");
      return;
    }
    const defaultLevel = selectedModelSpec?.default_thinking_level ?? thinkingLevels[0] ?? "";
    setThinkingLevel((current) => (current !== "" && thinkingLevels.includes(current) ? current : defaultLevel));
  }, [selectedModelSpec, supportsThinking, thinkingLevels]);

  const activeSession = sessions.find((session) => session.id === activeSessionID) ?? sessions[0];

  const touchSession = useCallback((sessionID: string, _firstUserText?: string) => {
    setSessions((current) =>
      current.map((session) => (session.id === sessionID ? { ...session, updatedAt: Date.now() } : session)),
    );
  }, []);

  const setSessionTitle = useCallback((sessionID: string, title: string) => {
    setSessions((current) =>
      current.map((session) => {
        if (session.id !== sessionID || session.title !== "Chat") {
          return session;
        }
        return { ...session, title: title.trim().slice(0, 48), updatedAt: Date.now() };
      }),
    );
  }, []);

  const newSession = () => {
    const session = createChatSession();
    setChatError("");
    setSessions((current) => [session, ...current]);
    setActiveSessionID(session.id);
    setResetToken((value) => value + 1);
  };

  const clearActiveSession = () => {
    if (activeSession == null) {
      return;
    }
    setChatError("");
    localStorage.removeItem(chatHistoryKey(activeSession.id));
    setSessions((current) => current.map((session) => (session.id === activeSession.id ? { ...session, title: "Chat", updatedAt: Date.now() } : session)));
    setResetToken((value) => value + 1);
  };

  const deleteActiveSession = () => {
    if (activeSession == null) {
      return;
    }
    setChatError("");
    localStorage.removeItem(chatHistoryKey(activeSession.id));
    setSessions((current) => {
      const next = current.filter((session) => session.id !== activeSession.id);
      const fallback = next[0] ?? createChatSession();
      setActiveSessionID(fallback.id);
      setResetToken((value) => value + 1);
      return next.length === 0 ? [fallback] : next;
    });
  };

  return (
    <Sheet modal={false} open={open} onOpenChange={onOpenChange}>
      <Button aria-pressed={open} onClick={() => onOpenChange(!open)} size="sm" type="button" variant={open ? "default" : "outline"}>
        <MessageCircle data-icon="inline-start" />
        OpenAI
      </Button>
      <SheetContent
        className={topDrawerContentClassName}
        onInteractOutside={(event) => event.preventDefault()}
        overlayClassName="pointer-events-none top-32 bg-transparent sm:top-24 lg:top-20"
        side="right"
      >
        <SheetHeader className="border-b px-5 py-4">
          <SheetTitle>OpenAI</SheetTitle>
          <SheetDescription>Send requests to this gateway through the OpenAI-compatible chat completions endpoint.</SheetDescription>
        </SheetHeader>
        <div className="grid min-h-0 flex-1 grid-cols-1 lg:grid-cols-[minmax(0,1fr)_280px]">
          <div className="flex min-h-0 flex-col">
            <div className="grid gap-3 border-b p-4 md:grid-cols-[minmax(0,1fr)_160px]">
              <SelectField label="Model" value={selectedModel} onChange={setSelectedModel} options={models.map((model) => model.id)} />
              {supportsTemperature ? <Field label="Temperature" value={temperature} onChange={setTemperature} /> : <div />}
              <div className="md:col-span-2">
                <div className="grid gap-3 md:grid-cols-[minmax(0,1fr)_160px]">
                  <ScrollableSelectField label="Voice" loading={voicesLoading} value={selectedVoice} onChange={setSelectedVoice} onOpen={loadVoices} options={playableVoices.map((voice) => voice.id)} />
                  <SwitchField label="Auto Speak" checked={autoSpeak} onChange={setAutoSpeakEnabled} />
                </div>
              </div>
              {supportsThinking ? (
                <div className="grid gap-3 md:col-span-2 md:grid-cols-[160px_minmax(0,1fr)]">
                  <Toggle label="Think" checked={thinkingEnabled} onChange={setThinkingEnabled} />
                  {thinkingLevels.length > 0 ? (
                    <SelectField label="Think Level" value={thinkingLevel} onChange={setThinkingLevel} options={thinkingLevels} />
                  ) : (
                    <div className="flex items-end text-xs text-muted-foreground">
                      <Brain className="mr-1 size-3" />
                      This model supports a thinking on/off switch.
                    </div>
                  )}
                </div>
              ) : null}
              <div className="md:col-span-2">
                <TextAreaField label="System Prompt" value={systemPrompt} onChange={setSystemPrompt} placeholder="Optional system instructions for this test chat." />
              </div>
            </div>
            {activeSession == null || selectedModel === "" ? (
              <EmptyMessage description="Create a session and select an LLM model before chatting." title="No chat target" />
            ) : (
              <ChatRuntime
                key={`${activeSession.id}:${resetToken}`}
                chatError={chatError}
                clearChatError={() => setChatError("")}
                model={selectedModel}
                onChatError={reportChatError}
                autoSpeak={autoSpeak && selectedVoice !== ""}
                sessionID={activeSession.id}
                setSessionTitle={setSessionTitle}
                systemPrompt={systemPrompt}
                thinking={supportsThinking ? { enabled: thinkingEnabled, level: thinkingLevel === "" ? undefined : thinkingLevel } : undefined}
                temperature={supportsTemperature ? Number.parseFloat(temperature) : undefined}
                touchSession={touchSession}
                voice={selectedVoice}
              />
            )}
          </div>
          <aside className="flex min-h-0 flex-col border-l bg-muted/30">
            <div className="flex items-center justify-between gap-2 border-b p-3">
              <div className="text-sm font-semibold">Sessions</div>
              <Button onClick={newSession} size="sm" type="button">
                <Plus className="size-4" />
                New
              </Button>
            </div>
            <div className="flex-1 overflow-y-auto p-2">
              {sessions.map((session) => (
                <button
                  className={cn(
                    "mb-1 flex w-full flex-col rounded-md px-3 py-2 text-left text-sm hover:bg-accent",
                    session.id === activeSessionID && "bg-accent text-accent-foreground",
                  )}
                  key={session.id}
                  onClick={() => {
                    setChatError("");
                    setActiveSessionID(session.id);
                    setResetToken((value) => value + 1);
                  }}
                  type="button"
                >
                  <span className="line-clamp-1 font-medium">{session.title}</span>
                  <span className="text-xs text-muted-foreground">{formatDate(new Date(session.updatedAt).toISOString())}</span>
                </button>
              ))}
            </div>
            <div className="grid gap-2 border-t p-3">
              <Button onClick={clearActiveSession} type="button" variant="outline">
                Clear Current
              </Button>
              <Button onClick={deleteActiveSession} type="button" variant="outline">
                <Trash2 className="size-4" />
                Delete Current
              </Button>
            </div>
          </aside>
        </div>
      </SheetContent>
    </Sheet>
  );
}

function ChatRuntime({
  autoSpeak,
  chatError,
  clearChatError,
  model,
  onChatError,
  sessionID,
  setSessionTitle,
  systemPrompt,
  thinking,
  temperature,
  touchSession,
  voice,
}: {
  autoSpeak: boolean;
  chatError: string;
  clearChatError: () => void;
  model: string;
  onChatError: (message: string) => void;
  sessionID: string;
  setSessionTitle: (sessionID: string, title: string) => void;
  systemPrompt: string;
  thinking?: ChatThinkingOptions;
  temperature?: number;
  touchSession: (sessionID: string, firstUserText?: string) => void;
  voice: string;
}): JSX.Element {
  const history = useMemo(() => createThreadHistoryAdapter(sessionID, touchSession), [sessionID, touchSession]);
  const speech = useMemo(() => (voice === "" ? undefined : createOpenAISpeechSynthesisAdapter({ onError: onChatError, voice })), [onChatError, voice]);
  const speakText = useCallback(
    (text: string) => {
      if (speech == null || text.trim() === "") {
        return;
      }
      void unlockBrowserAudio();
      speech.speak(text);
    },
    [speech],
  );
  const speakResponse = useCallback(
    (text: string) => {
      if (!autoSpeak) {
        return;
      }
      speakText(text);
    },
    [autoSpeak, speakText],
  );
  const adapter = useMemo(
    () => createOpenAIChatAdapter({ model, onChatError, onCompleteText: speakResponse, sessionID, setSessionTitle, systemPrompt, temperature, thinking }),
    [model, onChatError, sessionID, setSessionTitle, speakResponse, systemPrompt, temperature, thinking],
  );
  const runtime = useLocalRuntime(adapter, { adapters: { history, speech } });

  return (
    <AssistantRuntimeProvider runtime={runtime}>
      <ThreadPrimitive.Root className="flex min-h-0 flex-1 flex-col">
        <ThreadPrimitive.Viewport className="flex min-h-0 flex-1 flex-col gap-3 overflow-y-auto p-4">
          <AuiIf condition={(state) => state.thread.isEmpty}>
            <div className="m-auto max-w-sm text-center">
              <div className="text-sm font-medium">Ready to test {model}</div>
              <div className="mt-1 text-sm text-muted-foreground">Send a message to call /v1/chat/completions on this example service.</div>
            </div>
          </AuiIf>
          <ThreadPrimitive.Messages>{({ message }) => (message.role === "user" ? <UserChatMessage /> : <AssistantChatMessage onSpeak={speakText} />)}</ThreadPrimitive.Messages>
          <ThreadPrimitive.ViewportFooter className="sticky bottom-0 mt-auto bg-background pt-2">
            {chatError !== "" ? (
              <Alert className="mb-2 border-destructive/50 bg-destructive/5 text-destructive" variant="destructive">
                <AlertDescription className="flex items-start justify-between gap-3">
                  <span className="min-w-0 whitespace-pre-wrap break-words text-xs">{chatError}</span>
                  <Button aria-label="Dismiss chat error" className="h-6 shrink-0 px-2" onClick={clearChatError} size="sm" type="button" variant="ghost">
                    Dismiss
                  </Button>
                </AlertDescription>
              </Alert>
            ) : null}
            <ComposerPrimitive.Root className="rounded-lg border bg-background shadow-sm">
              <ComposerPrimitive.Input className="max-h-40 min-h-16 w-full resize-none bg-transparent px-3 py-3 text-sm outline-none" placeholder="Type a test message..." submitMode="ctrlEnter" />
              <div className="flex items-center justify-between border-t px-2 py-2">
                <div className="text-xs text-muted-foreground">Ctrl+Enter sends</div>
                <ComposerPrimitive.Send asChild>
                  <Button size="sm" type="submit">
                    <SendHorizontal className="size-4" />
                    Send
                  </Button>
                </ComposerPrimitive.Send>
              </div>
            </ComposerPrimitive.Root>
          </ThreadPrimitive.ViewportFooter>
        </ThreadPrimitive.Viewport>
      </ThreadPrimitive.Root>
    </AssistantRuntimeProvider>
  );
}

function UserChatMessage(): JSX.Element {
  const isEditing = useEditComposer({ optional: true, selector: (state: EditComposerState) => state.isEditing }) ?? false;

  return (
    <MessagePrimitive.Root className="group flex justify-end">
      <div className="flex max-w-[78%] flex-col items-end gap-1">
        {isEditing ? (
          <EditMessageComposer />
        ) : (
          <>
            <div className="whitespace-pre-wrap rounded-lg bg-primary px-3 py-2 text-sm text-primary-foreground">
              <MessagePrimitive.Parts />
            </div>
            <UserMessageActions />
          </>
        )}
      </div>
    </MessagePrimitive.Root>
  );
}

function AssistantChatMessage({ onSpeak }: { onSpeak: (text: string) => void }): JSX.Element {
  const message = useMessage();
  const text = threadMessageText(message);

  return (
    <MessagePrimitive.Root className="group flex justify-start">
      <div className="flex max-w-[82%] flex-col items-start gap-1">
        <div className="whitespace-pre-wrap rounded-lg bg-muted px-3 py-2 text-sm">
          <MessagePrimitive.Parts />
        </div>
        <AssistantMessageActions onSpeak={() => onSpeak(text)} speakDisabled={text.trim() === ""} />
      </div>
    </MessagePrimitive.Root>
  );
}

function UserMessageActions(): JSX.Element {
  return (
    <div className="flex items-center gap-1 opacity-0 transition-opacity group-hover:opacity-100 group-focus-within:opacity-100">
      <BranchPicker />
      <ActionBarPrimitive.Root hideWhenRunning>
        <ActionBarPrimitive.Edit asChild>
          <Button size="xs" type="button" variant="ghost">
            <Pencil className="size-3" />
            Edit
          </Button>
        </ActionBarPrimitive.Edit>
      </ActionBarPrimitive.Root>
    </div>
  );
}

function AssistantMessageActions({ onSpeak, speakDisabled }: { onSpeak: () => void; speakDisabled: boolean }): JSX.Element {
  return (
    <div className="flex items-center gap-1 opacity-0 transition-opacity group-hover:opacity-100 group-focus-within:opacity-100">
      <BranchPicker />
      <ActionBarPrimitive.Root hideWhenRunning>
        <Button disabled={speakDisabled} onClick={onSpeak} size="xs" type="button" variant="ghost">
          <Volume2 className="size-3" />
          Speak
        </Button>
        <ActionBarPrimitive.StopSpeaking asChild>
          <Button size="xs" type="button" variant="ghost">
            <VolumeX className="size-3" />
            Stop
          </Button>
        </ActionBarPrimitive.StopSpeaking>
        <ActionBarPrimitive.Reload asChild>
          <Button size="xs" type="button" variant="ghost">
            <RefreshCw className="size-3" />
            Regenerate
          </Button>
        </ActionBarPrimitive.Reload>
      </ActionBarPrimitive.Root>
    </div>
  );
}

function createOpenAISpeechSynthesisAdapter({
  onError,
  voice,
}: {
  onError: (message: string) => void;
  voice: string;
}): SpeechSynthesisAdapter {
  return {
    speak(text: string): SpeechSynthesisAdapter.Utterance {
      const subscribers = new Set<() => void>();
      const controller = new AbortController();
      let audio: HTMLAudioElement | null = null;
      let objectURL = "";
      let ended = false;

      const utterance: SpeechSynthesisAdapter.Utterance = {
        status: { type: "starting" },
        cancel: () => {
          controller.abort();
          if (audio != null) {
            audio.pause();
            audio.removeAttribute("src");
            audio.load();
          }
          finish("cancelled");
        },
        subscribe: (callback: () => void) => {
          if (utterance.status.type === "ended") {
            let cancelled = false;
            queueMicrotask(() => {
              if (!cancelled) {
                callback();
              }
            });
            return () => {
              cancelled = true;
            };
          }
          subscribers.add(callback);
          return () => {
            subscribers.delete(callback);
          };
        },
      };

      const notify = () => {
        subscribers.forEach((callback) => callback());
      };

      const finish = (reason: SpeechSynthesisAdapter.Status extends infer Status ? Status extends { type: "ended"; reason: infer Reason } ? Reason : never : never, error?: unknown) => {
        if (ended) {
          return;
        }
        ended = true;
        if (objectURL !== "") {
          URL.revokeObjectURL(objectURL);
          objectURL = "";
        }
        if (audio != null) {
          audio.remove();
        }
        utterance.status = error === undefined ? { type: "ended", reason } : { type: "ended", reason, error };
        notify();
      };

      const fail = (message: string, error?: unknown) => {
        console.error(message, error);
        onError(message);
        finish("error", error ?? new Error(message));
      };

      void (async () => {
        try {
          toast.info("Speech request started");
          const blob = await fetchSpeechAudioBlob({ input: text, signal: controller.signal, voice });
          toast.info(`Speech audio received (${blob.size} bytes)`);
          if (controller.signal.aborted) {
            finish("cancelled");
            return;
          }
          objectURL = URL.createObjectURL(blob);
          audio = new Audio(objectURL);
          audio.preload = "auto";
          audio.muted = false;
          audio.volume = 1;
          audio.setAttribute("playsinline", "true");
          audio.style.display = "none";
          document.body.append(audio);
          audio.addEventListener("ended", () => finish("finished"), { once: true });
          audio.addEventListener("error", () => fail("Speech playback failed", audio?.error ?? undefined), { once: true });
          utterance.status = { type: "running" };
          notify();
          await playAudioWithTimeout(audio);
          toast.success("Speech playback started");
        } catch (err) {
          if (isAbortError(err)) {
            finish("cancelled");
            return;
          }
          fail(`Speech playback failed: ${errorToMessage(err)}`, err);
        }
      })();

      return utterance;
    },
  };
}

let audioUnlockPromise: Promise<void> | null = null;

function requestWorkspaceAudioPlayback(): void {
  void unlockBrowserAudio();
  window.dispatchEvent(new Event(workspaceAudioPlaybackRequestEvent));
}

function unlockBrowserAudio(): Promise<void> {
  if (audioUnlockPromise != null) {
    return audioUnlockPromise;
  }
  audioUnlockPromise = (async () => {
    const AudioContextCtor = window.AudioContext ?? (window as Window & { webkitAudioContext?: typeof AudioContext }).webkitAudioContext;
    if (AudioContextCtor != null) {
      const ctx = new AudioContextCtor();
      if (ctx.state === "suspended") {
        await ctx.resume();
      }
      const source = ctx.createBufferSource();
      source.buffer = ctx.createBuffer(1, 1, 48000);
      source.connect(ctx.destination);
      source.start();
      setTimeout(() => void ctx.close(), 100);
    }
  })().catch((err: unknown) => {
    audioUnlockPromise = null;
    console.warn("Browser audio unlock failed", err);
  });
  return audioUnlockPromise;
}

function playAudioWithTimeout(audio: HTMLAudioElement): Promise<void> {
  return new Promise((resolve, reject) => {
    const timer = window.setTimeout(() => {
      reject(new Error("audio.play() timed out"));
    }, 10000);
    audio
      .play()
      .then(() => {
        window.clearTimeout(timer);
        resolve();
      })
      .catch((err: unknown) => {
        window.clearTimeout(timer);
        reject(err);
      });
  });
}

async function fetchSpeechAudioBlob({ input, signal, voice }: { input: string; signal: AbortSignal; voice: string }): Promise<Blob> {
  for (let attempt = 0; attempt < 2; attempt += 1) {
    try {
      const response = await getOpenAIClient().audio.speech.create(
        {
          input,
          model: "tts",
          response_format: "mp3",
          stream_format: "sse",
          voice,
        },
        {
          signal,
        },
      );
      if (response.ok) {
        toast.info("Speech stream response received");
        return readSpeechStreamAudioBlob(response);
      }
      const message = await responseErrorMessage(response);
      if (attempt === 0 && isTransientSpeechProxyError(message)) {
        toast.info("Speech request retrying");
        continue;
      }
      throw new Error(`Speech request failed: ${message}`);
    } catch (err) {
      if (isAbortError(err)) {
        throw err;
      }
      const message = errorToMessage(err);
      if (attempt === 0 && isTransientSpeechProxyError(message)) {
        toast.info("Speech request retrying");
        continue;
      }
      throw err;
    }
  }
  throw new Error("Speech request failed");
}

async function responseErrorMessage(response: Response): Promise<string> {
  const status = `HTTP ${response.status}${response.statusText === "" ? "" : ` ${response.statusText}`}`;
  const contentType = response.headers.get("content-type") ?? "";
  if (contentType.includes("application/json")) {
    try {
      const payload = (await response.json()) as unknown;
      const message = openAIErrorPayloadMessage(payload);
      return message === "" ? status : `${status}\n${message}`;
    } catch {
      return status;
    }
  }
  const body = (await response.text().catch(() => "")).trim();
  return body === "" ? status : `${status}\n${body}`;
}

async function readSpeechStreamAudioBlob(response: Response): Promise<Blob> {
  const contentType = response.headers.get("content-type") ?? "";
  if (!contentType.startsWith("text/event-stream")) {
    return response.blob();
  }
  if (response.body == null) {
    throw new Error("Speech stream response has no body");
  }

  const reader = response.body.getReader();
  const decoder = new TextDecoder();
  const chunks: BlobPart[] = [];
  let pending = "";
  let doneEvent = false;

  const processLine = (line: string) => {
    const trimmed = line.trim();
    if (trimmed === "" || !trimmed.startsWith("data:")) {
      return;
    }
    const data = trimmed.slice("data:".length).trim();
    const event = JSON.parse(data) as { audio?: string; done?: boolean; type?: string };
    switch (event.type) {
      case "speech.audio.delta":
        if (event.audio == null || event.audio === "") {
          throw new Error("Speech stream audio delta is empty");
        }
        chunks.push(base64ToArrayBuffer(event.audio));
        return;
      case "speech.audio.done":
        doneEvent = true;
        return;
      default:
        throw new Error(`Unexpected speech stream event: ${event.type ?? "unknown"}`);
    }
  };

  for (;;) {
    const { done, value } = await reader.read();
    pending += decoder.decode(value ?? new Uint8Array(), { stream: !done });
    for (;;) {
      const newline = pending.indexOf("\n");
      if (newline < 0) {
        break;
      }
      const line = pending.slice(0, newline);
      pending = pending.slice(newline + 1);
      processLine(line);
    }
    if (done) {
      break;
    }
  }
  if (pending.trim() !== "") {
    processLine(pending);
  }
  if (chunks.length === 0) {
    throw new Error("Speech stream returned no audio chunks");
  }
  if (!doneEvent) {
    throw new Error("Speech stream ended without done event");
  }
  return new Blob(chunks, { type: "audio/mpeg" });
}

function base64ToArrayBuffer(value: string): ArrayBuffer {
  const binary = atob(value);
  const bytes = new Uint8Array(binary.length);
  for (let i = 0; i < binary.length; i += 1) {
    bytes[i] = binary.charCodeAt(i);
  }
  return bytes.buffer.slice(bytes.byteOffset, bytes.byteOffset + bytes.byteLength) as ArrayBuffer;
}

function BranchPicker(): JSX.Element {
  return (
    <MessagePrimitive.If hasBranches>
      <BranchPickerPrimitive.Root className="flex h-6 items-center gap-1 rounded-md border bg-background px-1 text-xs text-muted-foreground">
        <BranchPickerPrimitive.Previous asChild>
          <Button aria-label="Previous branch" size="icon-xs" type="button" variant="ghost">
            <span aria-hidden="true">&lt;</span>
          </Button>
        </BranchPickerPrimitive.Previous>
        <span className="min-w-8 text-center">
          <BranchPickerPrimitive.Number />/<BranchPickerPrimitive.Count />
        </span>
        <BranchPickerPrimitive.Next asChild>
          <Button aria-label="Next branch" size="icon-xs" type="button" variant="ghost">
            <span aria-hidden="true">&gt;</span>
          </Button>
        </BranchPickerPrimitive.Next>
      </BranchPickerPrimitive.Root>
    </MessagePrimitive.If>
  );
}

function EditMessageComposer(): JSX.Element {
  return (
    <ComposerPrimitive.Root className="w-[min(560px,78vw)] rounded-lg border bg-background shadow-sm">
      <ComposerPrimitive.Input className="max-h-40 min-h-20 w-full resize-none bg-transparent px-3 py-3 text-sm outline-none" submitMode="ctrlEnter" />
      <div className="flex items-center justify-end gap-2 border-t px-2 py-2">
        <ComposerPrimitive.Cancel asChild>
          <Button size="sm" type="button" variant="outline">
            Cancel
          </Button>
        </ComposerPrimitive.Cancel>
        <ComposerPrimitive.Send asChild>
          <Button size="sm" type="submit">
            <SendHorizontal className="size-4" />
            Save & Send
          </Button>
        </ComposerPrimitive.Send>
      </div>
    </ComposerPrimitive.Root>
  );
}

function ChatHistoryTimeline({
  error,
  history,
  loading,
  onPlay,
}: {
  error: string;
  history: WorkspaceHistoryEntry[];
  loading: boolean;
  onPlay: (entry: WorkspaceHistoryEntry) => Promise<void>;
}): JSX.Element {
  if (loading && history.length === 0) {
    return <LoadingGrid />;
  }
  if (error !== "") {
    return <EmptyMessage description={error} title="History unavailable" />;
  }
  if (history.length === 0) {
    return <EmptyMessage description="No history is available for this conversation." title="No history" />;
  }
  return (
    <ScrollArea className="h-full">
      <div className="flex flex-col gap-3 p-4">
        {history.map((entry) => {
          const source = historyEntrySource(entry);
          return (
            <div className={cn("flex", entry.type === "gear" ? "justify-end" : "justify-start")} key={entry.id}>
              <div className={cn("max-w-[82%] rounded-md border px-3 py-2 text-sm", entry.type === "gear" ? "bg-primary text-primary-foreground" : "bg-muted")}>
                <div className={cn("mb-1 flex flex-wrap items-center gap-2 text-xs", entry.type === "gear" ? "text-primary-foreground/75" : "text-muted-foreground")}>
                  <Badge variant={entry.type === "gear" ? "secondary" : "outline"}>{entry.type}</Badge>
                  <span className="truncate">{source}</span>
                  <span>{formatDate(entry.created_at)}</span>
                </div>
                <div className="whitespace-pre-wrap break-words">{entry.text || entry.id}</div>
                <div className="mt-2 flex justify-end">
                  {entry.replay_available === true ? (
                    <Button onClick={() => void onPlay(entry)} size="xs" type="button" variant={entry.type === "gear" ? "secondary" : "outline"}>
                      <Play data-icon="inline-start" />
                      Play
                    </Button>
                  ) : (
                    <span className="text-xs opacity-70">No audio</span>
                  )}
                </div>
              </div>
            </div>
          );
        })}
      </div>
    </ScrollArea>
  );
}

function useWorkspaceHistory(workspaceName: string, order: "asc" | "desc"): {
  error: string;
  items: WorkspaceHistoryEntry[];
  lastUpdatedAt: string;
  loadNewer: (lastUpdatedAt?: string) => void;
  loading: boolean;
  refresh: () => void;
} {
  const [items, setItems] = useState<WorkspaceHistoryEntry[]>([]);
  const [nextCursor, setNextCursor] = useState("");
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(false);
  const [lastUpdatedAt, setLastUpdatedAt] = useState("");

  const load = useCallback(
    async (cursor: string, append: boolean, notifiedAt?: string) => {
      const normalizedWorkspace = workspaceName.trim();
      if (normalizedWorkspace === "") {
        setItems([]);
        setNextCursor("");
        setError("");
        setLastUpdatedAt("");
        return;
      }
      setLoading(true);
      setError("");
      try {
        const response = await listWorkspaceHistoryPage(normalizedWorkspace, cursor, order);
        const nextItems = response.items ?? response.data ?? [];
        setItems((current) => (append ? mergeHistoryEntries(current, nextItems, order) : nextItems));
        setNextCursor(response.next_cursor ?? "");
        setLastUpdatedAt(notifiedAt == null || notifiedAt === "" ? new Date().toISOString() : notifiedAt);
      } catch (err) {
        if (!append) {
          setItems([]);
        }
        setError(workspaceFeatureMessage(err));
      } finally {
        setLoading(false);
      }
    },
    [order, workspaceName],
  );

  useEffect(() => {
    void load("", false);
  }, [load]);

  return {
    error,
    items,
    lastUpdatedAt,
    loadNewer: (notifiedAt?: string) => {
      void load(nextCursor, nextCursor !== "", notifiedAt);
    },
    loading,
    refresh: () => {
      void load("", false);
    },
  };
}

function mergeHistoryEntries(current: WorkspaceHistoryEntry[], incoming: WorkspaceHistoryEntry[], order: "asc" | "desc"): WorkspaceHistoryEntry[] {
  const byID = new Map<string, WorkspaceHistoryEntry>();
  for (const item of current) {
    byID.set(item.id, item);
  }
  for (const item of incoming) {
    byID.set(item.id, item);
  }
  return Array.from(byID.values()).sort((left, right) => {
    const delta = new Date(left.created_at ?? 0).getTime() - new Date(right.created_at ?? 0).getTime();
    return order === "asc" ? delta : -delta;
  });
}

function historyEntrySource(entry: WorkspaceHistoryEntry): string {
  if (entry.type === "gear" && entry.gear_id != null && entry.gear_id !== "") {
    return `${entry.name} / ${entry.gear_id}`;
  }
  return entry.name;
}

function friendDisplayName(friend: FriendObject): string {
  return compactID(friend.peer_public_key ?? friend.id ?? friend.workspace_name ?? "friend");
}

function groupDisplayName(group: FriendGroupObject): string {
  return group.name || compactID(group.id ?? group.workspace_name ?? "group");
}

function friendChatTarget(friend: FriendObject): SocialChatTarget {
  return {
    id: friend.id ?? friend.peer_public_key ?? friend.workspace_name ?? "",
    kind: "friend",
    title: friendDisplayName(friend),
    workspaceName: friend.workspace_name ?? "",
  };
}

function groupChatTarget(group: FriendGroupObject): SocialChatTarget {
  return {
    id: group.id ?? group.name ?? group.workspace_name ?? "",
    kind: "group",
    title: groupDisplayName(group),
    workspaceName: group.workspace_name ?? "",
  };
}

function socialTargetKey(target: SocialChatTarget): string {
  return `${target.kind}:${target.id}:${target.workspaceName}`;
}

function listFriendsPage(cursor: string): Promise<PageResponse<FriendObject>> {
  return expectData(listPeerFriends({ query: pageQuery(cursor) })) as Promise<PageResponse<FriendObject>>;
}

function getFriendInviteToken(): Promise<FriendInviteTokenGetResponse> {
  return expectData(getPeerFriendInviteToken()) as Promise<FriendInviteTokenGetResponse>;
}

function createFriendInviteToken(): Promise<FriendInviteTokenGetResponse> {
  return expectData(createPeerFriendInviteToken()) as Promise<FriendInviteTokenGetResponse>;
}

function clearFriendInviteToken(): Promise<unknown> {
  return expectData(clearPeerFriendInviteToken());
}

function addFriendByInviteToken(inviteToken: string): Promise<FriendObject> {
  return expectData(addPeerFriend({ body: { invite_token: inviteToken } })) as Promise<FriendObject>;
}

function deleteFriend(id: string): Promise<FriendObject> {
  return expectData(deletePeerFriend({ path: { id } })) as Promise<FriendObject>;
}

function listFriendGroupsPage(cursor: string): Promise<PageResponse<FriendGroupObject>> {
  return expectData(listPeerFriendGroups({ query: pageQuery(cursor) })) as Promise<PageResponse<FriendGroupObject>>;
}

function createFriendGroup(name: string, description: string): Promise<FriendGroupObject> {
  const body = description === "" ? { name } : { name, description };
  return expectData(createPeerFriendGroup({ body })) as Promise<FriendGroupObject>;
}

function getFriendGroup(id: string): Promise<FriendGroupObject> {
  return expectData(getPeerFriendGroup({ path: { id } })) as Promise<FriendGroupObject>;
}

function joinFriendGroupByInviteToken(inviteToken: string): Promise<{ group: FriendGroupObject; member: FriendGroupMemberObject }> {
  return expectData(joinPeerFriendGroup({ body: { invite_token: inviteToken } })) as Promise<{ group: FriendGroupObject; member: FriendGroupMemberObject }>;
}

function getFriendGroupInviteToken(id: string): Promise<FriendGroupInviteTokenGetResponse> {
  return expectData(getPeerFriendGroupInviteToken({ path: { id } })) as Promise<FriendGroupInviteTokenGetResponse>;
}

function createFriendGroupInviteToken(id: string): Promise<FriendGroupInviteTokenGetResponse> {
  return expectData(createPeerFriendGroupInviteToken({ path: { id } })) as Promise<FriendGroupInviteTokenGetResponse>;
}

function clearFriendGroupInviteToken(id: string): Promise<unknown> {
  return expectData(clearPeerFriendGroupInviteToken({ path: { id } }));
}

function listFriendGroupMembersPage(id: string, cursor: string): Promise<PageResponse<FriendGroupMemberObject>> {
  if (id.trim() === "") {
    return Promise.resolve({ has_next: false, items: [] });
  }
  return expectData(listPeerFriendGroupMembers({ path: { id }, query: pageQuery(cursor) })) as Promise<PageResponse<FriendGroupMemberObject>>;
}

function addFriendGroupMember(id: string, peerPublicKey: string, role: FriendGroupMemberMutableRole): Promise<FriendGroupMemberObject> {
  return expectData(addPeerFriendGroupMember({ body: { friend_group_id: id, peer_public_key: peerPublicKey, role }, path: { id } })) as Promise<FriendGroupMemberObject>;
}

function updateFriendGroupMember(id: string, memberID: string, role: FriendGroupMemberMutableRole): Promise<FriendGroupMemberObject> {
  return expectData(putPeerFriendGroupMember({ body: { friend_group_id: id, id: memberID, role }, path: { id, member_id: memberID } })) as Promise<FriendGroupMemberObject>;
}

function deleteFriendGroupMember(id: string, memberID: string): Promise<FriendGroupMemberObject> {
  return expectData(deletePeerFriendGroupMember({ path: { id, member_id: memberID } })) as Promise<FriendGroupMemberObject>;
}

function listWorkspaceHistoryPage(workspaceName: string, cursor: string, order: "asc" | "desc"): Promise<PageResponse<WorkspaceHistoryEntry>> {
  return expectData(listPeerWorkspaceHistory({ path: { workspace_name: workspaceName }, query: { ...pageQuery(cursor), order } })) as Promise<PageResponse<WorkspaceHistoryEntry>>;
}

async function playWorkspaceHistoryAsset(workspaceName: string, historyID: string): Promise<void> {
  const blob = await expectData(getPeerWorkspaceHistoryAudio({ path: { history_id: historyID, workspace_name: workspaceName } })) as Blob;
  const url = URL.createObjectURL(blob);
  const audio = new Audio(url);
  audio.preload = "auto";
  audio.setAttribute("playsinline", "true");
  audio.addEventListener("ended", () => URL.revokeObjectURL(url), { once: true });
  audio.addEventListener("error", () => URL.revokeObjectURL(url), { once: true });
  await unlockBrowserAudio();
  await playAudioWithTimeout(audio);
}

function usePagedList<T>(loadPage: (cursor: string) => Promise<PageResponse<T>>): {
  error: string;
  next: () => void;
  page: PagedState<T>;
  previous: () => void;
  refresh: () => void;
} {
  const [page, setPage] = useState<PagedState<T>>({
    cursors: [""],
    error: "",
    hasNext: false,
    items: [],
    loading: true,
    nextCursor: "",
  });

  const load = useCallback(
    async (cursor: string, cursors: string[]) => {
      setPage((current) => ({ ...current, error: "", loading: true }));
      try {
        const response = await loadPage(cursor);
        setPage({
          cursors,
          error: "",
          hasNext: response.has_next === true && response.next_cursor != null && response.next_cursor !== "",
          items: response.items ?? response.data ?? [],
          loading: false,
          nextCursor: response.next_cursor ?? "",
        });
      } catch (err) {
        setPage((current) => ({ ...current, error: toMessage(err), loading: false }));
      }
    },
    [loadPage],
  );

  useEffect(() => {
    void load("", [""]);
  }, [load]);

  return {
    error: page.error,
    next: () => {
      if (!page.hasNext || page.nextCursor === "") {
        return;
      }
      void load(page.nextCursor, [...page.cursors, page.nextCursor]);
    },
    page,
    previous: () => {
      if (page.cursors.length <= 1) {
        return;
      }
      const cursors = page.cursors.slice(0, -1);
      void load(cursors[cursors.length - 1] ?? "", cursors);
    },
    refresh: () => {
      const cursor = page.cursors[page.cursors.length - 1] ?? "";
      void load(cursor, page.cursors);
    },
  };
}

function PageAction({ canNext, canPrevious, loading, onNext, onPrevious, onRefresh, pageIndex }: { canNext: boolean; canPrevious: boolean; loading: boolean; onNext: () => void; onPrevious: () => void; onRefresh: () => void; pageIndex: number }): JSX.Element {
  return (
    <div className="flex items-center gap-2 text-sm">
      <span className="text-muted-foreground">Page {pageIndex}</span>
      <Button disabled={loading} onClick={onRefresh} size="sm" type="button" variant="outline">
        <RefreshCw className={cn("size-4", loading && "animate-spin")} />
      </Button>
      <Button disabled={loading || !canPrevious} onClick={onPrevious} size="sm" type="button" variant="outline">
        Prev
      </Button>
      <Button disabled={loading || !canNext} onClick={onNext} size="sm" type="button" variant="outline">
        Next
      </Button>
    </div>
  );
}

function PagedSimpleTable<T>({
  columns,
  empty,
  loadPage,
  row,
  title,
}: {
  columns: string[];
  empty: string;
  loadPage: (cursor: string) => Promise<PageResponse<T>>;
  row: (item: T) => string[];
  title: string;
}): JSX.Element {
  const pager = usePagedList(loadPage);
  return (
    <div className="space-y-3">
      {pager.error !== "" ? (
        <Alert variant="destructive">
          <AlertDescription>{pager.error}</AlertDescription>
        </Alert>
      ) : null}
      <SimpleTable
        action={<PageAction canNext={pager.page.hasNext} canPrevious={pager.page.cursors.length > 1} loading={pager.page.loading} onNext={pager.next} onPrevious={pager.previous} onRefresh={pager.refresh} pageIndex={pager.page.cursors.length} />}
        columns={columns}
        empty={pager.page.loading ? "Loading" : empty}
        rows={pager.page.items.map(row)}
        title={title}
      />
    </div>
  );
}

function WorkspacesPanel(): JSX.Element {
  const loadPage = useCallback(async (cursor: string) => {
    const page = await listPeerResourcePage("workspaces", cursor);
    const items = page.items ?? page.data;
    if (items == null) {
      return page;
    }
    return { ...page, items: sortWorkspacesByActivity(items), data: undefined };
  }, []);
  return (
    <PagedSimpleTable
      columns={["Display name", "Workspace ID", "Workflow", "Last active", "Updated"]}
      empty="No workspaces"
      loadPage={loadPage}
      row={(item) => [stringField(item, "display_name") || "-", stringField(item, "name"), stringField(item, "workflow_name"), formatDate(stringField(item, "last_active_at")), formatDate(stringField(item, "updated_at"))]}
      title="Workspaces"
    />
  );
}

function WorkflowsPanel(): JSX.Element {
  const loadPage = useCallback((cursor: string) => listPeerResourcePage("workflows", cursor), []);
  return (
    <PagedSimpleTable
      columns={["Name", "Kind", "API Version"]}
      empty="No workflows"
      loadPage={loadPage}
      row={(item) => {
        const metadata = objectField(item, "metadata");
        return [stringField(metadata, "name"), stringField(item, "kind"), stringField(item, "apiVersion")];
      }}
      title="Workflows"
    />
  );
}

function CredentialsPanel(): JSX.Element {
  const loadPage = useCallback((cursor: string) => listPeerResourcePage("credentials", cursor), []);
  return (
    <PagedSimpleTable
      columns={["Name", "Provider", "Method", "Description", "Updated"]}
      empty="No credentials"
      loadPage={loadPage}
      row={(item) => [stringField(item, "name"), stringField(item, "provider"), stringField(item, "method"), stringField(item, "description"), formatDate(stringField(item, "updated_at"))]}
      title="Credentials"
    />
  );
}

function OverviewPanel({ modelCount, wallet }: { modelCount: number; wallet: WalletResource | null }): JSX.Element {
  return (
    <div className="grid max-w-6xl gap-4 md:grid-cols-2">
      <Card>
        <CardHeader>
          <CardTitle>Wallet</CardTitle>
        </CardHeader>
        <CardContent>
          {wallet == null ? (
            <EmptyMessage description="No wallet is visible for this context." title="No wallet" />
          ) : (
            <div className="grid gap-3 text-sm">
              <div className="grid grid-cols-2 gap-3">
                <div>
                  <div className="text-xs text-muted-foreground">Points</div>
                  <div className="text-2xl font-semibold">{wallet.point_balance}</div>
                </div>
                <div>
                  <div className="text-xs text-muted-foreground">Tokens</div>
                  <div className="text-2xl font-semibold">{wallet.token_balance}</div>
                </div>
              </div>
              <div>
                <div className="text-xs text-muted-foreground">Wallet ID</div>
                <div className="break-all font-mono text-xs">{wallet.id}</div>
              </div>
              <div>
                <div className="text-xs text-muted-foreground">Updated</div>
                <div>{formatDate(wallet.updated_at)}</div>
              </div>
            </div>
          )}
        </CardContent>
      </Card>
      <Card>
        <CardHeader>
          <CardTitle>Gateway</CardTitle>
        </CardHeader>
        <CardContent>
          <div className="grid gap-3 text-sm">
            <div>
              <div className="text-xs text-muted-foreground">Models</div>
              <div className="text-2xl font-semibold">{modelCount}</div>
            </div>
            <div className="text-muted-foreground">ACL-controlled resources are listed in the resource sections. Peer-owned singleton state is shown here.</div>
          </div>
        </CardContent>
      </Card>
    </div>
  );
}

function WalletTransactionsPanel(): JSX.Element {
  const loadPage = useCallback((cursor: string) => listWalletTransactionsPage(cursor), []);
  const pager = usePagedList(loadPage);
  const inspect = async (id: string): Promise<void> => {
    const tx = await getWalletTransaction(id);
    toast.success("Transaction loaded", { description: `${tx.reason}: points ${signedNumber(tx.point_delta)}, tokens ${signedNumber(tx.token_delta)}` });
  };
  return (
    <div className="flex max-w-6xl flex-col gap-3">
      {pager.error !== "" ? (
        <Alert variant="destructive">
          <AlertDescription>{pager.error}</AlertDescription>
        </Alert>
      ) : null}
      <Card>
        <CardHeader className="flex flex-row items-center justify-between gap-3">
          <CardTitle>Wallet Transactions</CardTitle>
          <PageAction canNext={pager.page.hasNext} canPrevious={pager.page.cursors.length > 1} loading={pager.page.loading} onNext={pager.next} onPrevious={pager.previous} onRefresh={pager.refresh} pageIndex={pager.page.cursors.length} />
        </CardHeader>
        <CardContent>
          {pager.page.items.length === 0 ? (
            <EmptyMessage description="No wallet transactions are visible for this context." title="No transactions" />
          ) : (
            <div className="rounded-md border">
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>ID</TableHead>
                    <TableHead>Points</TableHead>
                    <TableHead>Tokens</TableHead>
                    <TableHead>Reason</TableHead>
                    <TableHead>Created</TableHead>
                    <TableHead className="w-24 text-right">Actions</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {pager.page.items.map((tx) => (
                    <TableRow key={tx.id}>
                      <TableCell className="font-mono text-xs">{tx.id}</TableCell>
                      <TableCell>{signedNumber(tx.point_delta)}</TableCell>
                      <TableCell>{signedNumber(tx.token_delta)}</TableCell>
                      <TableCell>{tx.reason}</TableCell>
                      <TableCell className="text-muted-foreground">{formatDate(tx.created_at)}</TableCell>
                      <TableCell className="text-right">
                        <Button
                          onClick={() => {
                            void inspect(tx.id).catch((err: unknown) => toast.error("Transaction request failed", { description: toMessage(err) }));
                          }}
                          size="sm"
                          type="button"
                          variant="outline"
                        >
                          Inspect
                        </Button>
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </div>
          )}
        </CardContent>
      </Card>
    </div>
  );
}

function PetsPanel(): JSX.Element {
  const loadPage = useCallback((cursor: string) => listPetsPage(cursor), []);
  const pager = usePagedList(loadPage);
  const [dialog, setDialog] = useState<PetDialogState | null>(null);
  const refreshAfter = async (message: string): Promise<void> => {
    toast.success(message);
    await pager.refresh();
  };
  return (
    <div className="flex max-w-6xl flex-col gap-3">
      {pager.error !== "" ? (
        <Alert variant="destructive">
          <AlertDescription>{pager.error}</AlertDescription>
        </Alert>
      ) : null}
      <Card>
        <CardHeader className="flex flex-row items-center justify-between gap-3">
          <CardTitle>Pets</CardTitle>
          <div className="flex items-center gap-2">
            <Button onClick={() => setDialog({ kind: "adopt" })} size="sm" type="button">
              <Plus data-icon="inline-start" />
              Adopt
            </Button>
            <PageAction canNext={pager.page.hasNext} canPrevious={pager.page.cursors.length > 1} loading={pager.page.loading} onNext={pager.next} onPrevious={pager.previous} onRefresh={pager.refresh} pageIndex={pager.page.cursors.length} />
          </div>
        </CardHeader>
        <CardContent>
          {pager.page.items.length === 0 ? (
            <EmptyMessage description="No pets are visible for this context." title="No pets" />
          ) : (
            <div className="rounded-md border">
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>Name</TableHead>
                    <TableHead>Life</TableHead>
                    <TableHead>Ability</TableHead>
                    <TableHead>Updated</TableHead>
                    <TableHead className="w-[320px] text-right">Actions</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {pager.page.items.map((pet) => (
                    <TableRow key={pet.id}>
                      <TableCell>
                        <div className="font-medium">{pet.name}</div>
                        <div className="font-mono text-xs text-muted-foreground">{pet.id}</div>
                      </TableCell>
                      <TableCell>{statsSummary(pet.life, ["mood", "health", "energy"])}</TableCell>
                      <TableCell>{statsSummary(pet.ability, ["level", "exp", "stamina"])}</TableCell>
                      <TableCell className="text-muted-foreground">{formatDate(pet.updated_at)}</TableCell>
                      <TableCell>
                        <div className="flex flex-wrap justify-end gap-2">
                          <Button onClick={() => setDialog({ kind: "rename", pet })} size="sm" type="button" variant="outline">
                            <Pencil data-icon="inline-start" />
                            Rename
                          </Button>
                          <Button onClick={() => setDialog({ kind: "feed", pet })} size="sm" type="button" variant="outline">
                            Feed
                          </Button>
                          <Button onClick={() => setDialog({ kind: "wash", pet })} size="sm" type="button" variant="outline">
                            Wash
                          </Button>
                          <Button onClick={() => setDialog({ kind: "play", pet })} size="sm" type="button" variant="outline">
                            Play
                          </Button>
                          <PetDeleteAction
                            onDelete={async () => {
                              await deletePet(pet.id);
                              await refreshAfter("Pet deleted");
                            }}
                          />
                        </div>
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </div>
          )}
        </CardContent>
      </Card>
      <PetDialog
        onClose={() => setDialog(null)}
        onSubmit={(state, value) =>
          submitPetDialog(state, value)
            .then(() => refreshAfter(petDialogSuccess(state.kind)))
            .then(() => setDialog(null))
        }
        state={dialog}
      />
    </div>
  );
}

function RewardsPanel(): JSX.Element {
  const loadPage = useCallback((cursor: string) => listRewardsPage(cursor), []);
  const pager = usePagedList(loadPage);
  const [claimOpen, setClaimOpen] = useState(false);
  const inspect = async (id: string): Promise<void> => {
    const reward = await getReward(id);
    toast.success("Reward loaded", { description: `${reward.point_amount} points for ${reward.badge_id}` });
  };
  return (
    <div className="flex max-w-6xl flex-col gap-3">
      {pager.error !== "" ? (
        <Alert variant="destructive">
          <AlertDescription>{pager.error}</AlertDescription>
        </Alert>
      ) : null}
      <Card>
        <CardHeader className="flex flex-row items-center justify-between gap-3">
          <CardTitle>Rewards</CardTitle>
          <div className="flex items-center gap-2">
            <Button onClick={() => setClaimOpen(true)} size="sm" type="button">
              <Gift data-icon="inline-start" />
              Claim
            </Button>
            <PageAction canNext={pager.page.hasNext} canPrevious={pager.page.cursors.length > 1} loading={pager.page.loading} onNext={pager.next} onPrevious={pager.previous} onRefresh={pager.refresh} pageIndex={pager.page.cursors.length} />
          </div>
        </CardHeader>
        <CardContent>
          {pager.page.items.length === 0 ? (
            <EmptyMessage description="No rewards are visible for this context." title="No rewards" />
          ) : (
            <div className="rounded-md border">
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>ID</TableHead>
                    <TableHead>Prompt</TableHead>
                    <TableHead>Badge</TableHead>
                    <TableHead>Points</TableHead>
                    <TableHead>Created</TableHead>
                    <TableHead className="w-24 text-right">Actions</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {pager.page.items.map((reward) => (
                    <TableRow key={reward.id}>
                      <TableCell className="font-mono text-xs">{reward.id}</TableCell>
                      <TableCell className="max-w-sm truncate">{reward.prompt}</TableCell>
                      <TableCell>{reward.badge_id}</TableCell>
                      <TableCell>{reward.point_amount}</TableCell>
                      <TableCell className="text-muted-foreground">{formatDate(reward.created_at)}</TableCell>
                      <TableCell className="text-right">
                        <Button
                          onClick={() => {
                            void inspect(reward.id).catch((err: unknown) => toast.error("Reward request failed", { description: toMessage(err) }));
                          }}
                          size="sm"
                          type="button"
                          variant="outline"
                        >
                          Inspect
                        </Button>
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </div>
          )}
        </CardContent>
      </Card>
      <PromptDialog
        description="Submit a reward prompt through the current peer context."
        label="Prompt"
        onClose={() => setClaimOpen(false)}
        onSubmit={(prompt) =>
          claimReward(prompt)
            .then(() => {
              toast.success("Reward claimed");
              return pager.refresh();
            })
            .then(() => setClaimOpen(false))
        }
        open={claimOpen}
        submitLabel="Claim"
        title="Claim Reward"
      />
    </div>
  );
}

type PetDialogState =
  | { kind: "adopt" }
  | { kind: "feed"; pet: PetResource }
  | { kind: "play"; pet: PetResource }
  | { kind: "rename"; pet: PetResource }
  | { kind: "wash"; pet: PetResource };

function PetDialog({ onClose, onSubmit, state }: { onClose: () => void; onSubmit: (state: PetDialogState, value: string) => Promise<void>; state: PetDialogState | null }): JSX.Element {
  const [value, setValue] = useState("");
  const [saving, setSaving] = useState(false);
  const open = state != null;

  useEffect(() => {
    if (state == null) {
      setValue("");
      return;
    }
    setValue(state.kind === "rename" ? state.pet.name : "");
  }, [state]);

  if (state == null) {
    return <Dialog open={false} />;
  }

  const title = petDialogTitle(state);
  const label = state.kind === "adopt" || state.kind === "rename" ? "Name" : "Prompt";
  const submitLabel = state.kind === "adopt" ? "Adopt" : state.kind === "rename" ? "Save" : title;
  const multiline = state.kind === "feed" || state.kind === "wash" || state.kind === "play";

  const submit = async (): Promise<void> => {
    setSaving(true);
    try {
      await onSubmit(state, value);
    } catch (err) {
      toast.error(`${title} failed`, { description: toMessage(err) });
    } finally {
      setSaving(false);
    }
  };

  return (
    <Dialog open={open} onOpenChange={(next) => (!next ? onClose() : undefined)}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{title}</DialogTitle>
          <DialogDescription>{petDialogDescription(state)}</DialogDescription>
        </DialogHeader>
        <FieldGroup>
          <ShadField>
            <FieldLabel htmlFor="pet-dialog-value">{label}</FieldLabel>
            {multiline ? <Textarea id="pet-dialog-value" onChange={(event) => setValue(event.target.value)} rows={4} value={value} /> : <Input id="pet-dialog-value" onChange={(event) => setValue(event.target.value)} value={value} />}
          </ShadField>
        </FieldGroup>
        <DialogFooter>
          <Button disabled={saving} onClick={onClose} type="button" variant="outline">
            Cancel
          </Button>
          <Button disabled={saving || value.trim() === ""} onClick={() => void submit()} type="button">
            {submitLabel}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

function PromptDialog({
  description,
  label,
  onClose,
  onSubmit,
  open,
  submitLabel,
  title,
}: {
  description: string;
  label: string;
  onClose: () => void;
  onSubmit: (value: string) => Promise<void>;
  open: boolean;
  submitLabel: string;
  title: string;
}): JSX.Element {
  const [value, setValue] = useState("");
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    if (!open) {
      setValue("");
    }
  }, [open]);

  const submit = async (): Promise<void> => {
    setSaving(true);
    try {
      await onSubmit(value);
    } catch (err) {
      toast.error(`${title} failed`, { description: toMessage(err) });
    } finally {
      setSaving(false);
    }
  };

  return (
    <Dialog open={open} onOpenChange={(next) => (!next ? onClose() : undefined)}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{title}</DialogTitle>
          <DialogDescription>{description}</DialogDescription>
        </DialogHeader>
        <FieldGroup>
          <ShadField>
            <FieldLabel htmlFor="prompt-dialog-value">{label}</FieldLabel>
            <Textarea id="prompt-dialog-value" onChange={(event) => setValue(event.target.value)} rows={4} value={value} />
          </ShadField>
        </FieldGroup>
        <DialogFooter>
          <Button disabled={saving} onClick={onClose} type="button" variant="outline">
            Cancel
          </Button>
          <Button disabled={saving || value.trim() === ""} onClick={() => void submit()} type="button">
            {submitLabel}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

function PetDeleteAction({ onDelete }: { onDelete: () => Promise<void> }): JSX.Element {
  const [open, setOpen] = useState(false);
  const [saving, setSaving] = useState(false);
  const confirm = async (): Promise<void> => {
    setSaving(true);
    try {
      await onDelete();
      setOpen(false);
    } finally {
      setSaving(false);
    }
  };
  return (
    <AlertDialog open={open} onOpenChange={setOpen}>
      <Button onClick={() => setOpen(true)} size="sm" type="button" variant="destructive">
        <Trash2 data-icon="inline-start" />
        Delete
      </Button>
      <AlertDialogContent size="sm">
        <AlertDialogHeader>
          <AlertDialogTitle>Delete Pet</AlertDialogTitle>
          <AlertDialogDescription>This removes the pet owned by the current peer.</AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel disabled={saving}>Cancel</AlertDialogCancel>
          <AlertDialogAction disabled={saving} onClick={(event) => {
            event.preventDefault();
            void confirm().catch((err: unknown) => toast.error("Pet delete failed", { description: toMessage(err) }));
          }} variant="destructive">
            Delete
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}

function petDialogTitle(state: PetDialogState): string {
  switch (state.kind) {
    case "adopt":
      return "Adopt Pet";
    case "feed":
      return "Feed Pet";
    case "play":
      return "Play With Pet";
    case "rename":
      return "Rename Pet";
    case "wash":
      return "Wash Pet";
  }
}

function petDialogDescription(state: PetDialogState): string {
  switch (state.kind) {
    case "adopt":
      return "Create a pet for the current peer.";
    case "rename":
      return `Update ${state.pet.name}.`;
    default:
      return `Send an action prompt for ${state.pet.name}.`;
  }
}

function petDialogSuccess(kind: PetDialogState["kind"]): string {
  switch (kind) {
    case "adopt":
      return "Pet adopted";
    case "feed":
      return "Pet fed";
    case "play":
      return "Pet played";
    case "rename":
      return "Pet renamed";
    case "wash":
      return "Pet washed";
  }
}

function submitPetDialog(state: PetDialogState, value: string): Promise<PetResource> {
  const trimmed = value.trim();
  switch (state.kind) {
    case "adopt":
      return adoptPet(trimmed);
    case "feed":
      return runPetAction(state.pet.id, "feed", trimmed);
    case "play":
      return runPetAction(state.pet.id, "play", trimmed);
    case "rename":
      return updatePet(state.pet.id, trimmed);
    case "wash":
      return runPetAction(state.pet.id, "wash", trimmed);
  }
}

function statsSummary(stats: PetStats, keys: string[]): string {
  return keys.map((key) => `${key}: ${stats[key] ?? 0}`).join(" / ");
}

function signedNumber(value: number): string {
  return value > 0 ? `+${value}` : String(value);
}

function ModelsPanel({ initialModels }: { initialModels: ModelSpec[] }): JSX.Element {
  const loadPage = useCallback((cursor: string) => listModelsPage(cursor), []);
  const pager = usePagedList(loadPage);
  const models = pager.page.items.length === 0 && pager.page.loading ? initialModels : pager.page.items;
  return (
    <div className="max-w-6xl space-y-3">
      {pager.error !== "" ? (
        <Alert variant="destructive">
          <AlertDescription>{pager.error}</AlertDescription>
        </Alert>
      ) : null}
      <Card>
        <CardHeader className="flex flex-row items-center justify-between gap-3">
          <CardTitle>Models</CardTitle>
          <PageAction canNext={pager.page.hasNext} canPrevious={pager.page.cursors.length > 1} loading={pager.page.loading} onNext={pager.next} onPrevious={pager.previous} onRefresh={pager.refresh} pageIndex={pager.page.cursors.length} />
        </CardHeader>
        <CardContent>
          {models.length === 0 ? (
            <EmptyMessage description="No model resources are visible for this context." title="No models" />
          ) : (
            <div className="rounded-md border">
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>ID</TableHead>
                    <TableHead>Kind</TableHead>
                    <TableHead>Provider</TableHead>
                    <TableHead>Think</TableHead>
                    <TableHead>Source</TableHead>
                    <TableHead>Updated</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {models.map((model) => (
                    <TableRow key={model.id}>
                      <TableCell className="font-mono text-xs font-medium">{model.id}</TableCell>
                      <TableCell>{model.kind ?? "-"}</TableCell>
                      <TableCell>{model.provider == null ? "-" : `${model.provider.kind}/${model.provider.name}`}</TableCell>
                      <TableCell>{model.support_thinking === true ? <Badge variant="outline">{model.thinking_param || "on"}</Badge> : "-"}</TableCell>
                      <TableCell>{model.source ?? "-"}</TableCell>
                      <TableCell className="text-muted-foreground">{formatDate(model.updated_at)}</TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </div>
          )}
        </CardContent>
      </Card>
    </div>
  );
}

function VoicesPanel(): JSX.Element {
  const loadPage = useCallback((cursor: string) => listVoicesPage(cursor), []);
  const pager = usePagedList(loadPage);

  return (
    <SimpleTable
      action={<PageAction canNext={pager.page.hasNext} canPrevious={pager.page.cursors.length > 1} loading={pager.page.loading} onNext={pager.next} onPrevious={pager.previous} onRefresh={pager.refresh} pageIndex={pager.page.cursors.length} />}
      columns={["ID", "Provider", "Name", "Source", "Updated"]}
      empty={pager.page.loading ? "Loading" : pager.error || "No voices"}
      rows={pager.page.items.map((item) => [compactID(item.id), `${item.provider.kind}/${item.provider.name}`, item.name ?? "", item.source, formatDate(item.updated_at)])}
      title="Voices"
    />
  );
}

function SimpleTable({
  action,
  columns,
  empty,
  rows,
  title,
}: {
  action?: JSX.Element;
  columns: string[];
  empty: string;
  rows: string[][];
  title: string;
}): JSX.Element {
  return (
    <Card className="max-w-6xl">
      <CardHeader className="flex flex-row items-center justify-between gap-3">
        <CardTitle>{title}</CardTitle>
        {action}
      </CardHeader>
      <CardContent>
        {rows.length === 0 ? (
          <EmptyMessage description={empty} title={empty} />
        ) : (
          <div className="rounded-md border">
            <Table>
              <TableHeader>
                <TableRow>
                  {columns.map((column) => (
                    <TableHead key={column}>{column}</TableHead>
                  ))}
                </TableRow>
              </TableHeader>
              <TableBody>
                {rows.map((row) => (
                  <TableRow key={row.join(":")}>
                    {row.map((cell, index) => (
                      <TableCell className={index === 0 ? "font-medium" : "text-muted-foreground"} key={`${index}:${cell}`}>
                        {cell || "-"}
                      </TableCell>
                    ))}
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </div>
        )}
      </CardContent>
    </Card>
  );
}

function EmptyMessage({ description, title }: { description: string; title: string }): JSX.Element {
  return (
    <Empty className="min-h-56 border">
      <EmptyHeader>
        <EmptyTitle>{title}</EmptyTitle>
        <EmptyDescription>{description}</EmptyDescription>
      </EmptyHeader>
    </Empty>
  );
}

function Field({ label, onChange, type = "text", value }: { label: string; onChange: (value: string) => void; type?: string; value: string }): JSX.Element {
  return (
    <div className="flex flex-col gap-1.5">
      <Label>{label}</Label>
      <Input onChange={(event) => onChange(event.target.value)} type={type} value={value} />
    </div>
  );
}

function TextAreaField({ label, onChange, placeholder, value }: { label: string; onChange: (value: string) => void; placeholder?: string; value: string }): JSX.Element {
  return (
    <div className="flex flex-col gap-1.5">
      <Label>{label}</Label>
      <Textarea className="min-h-20 resize-y" onChange={(event) => onChange(event.target.value)} placeholder={placeholder} value={value} />
    </div>
  );
}

function SelectField({ label, onChange, options, value }: { label: string; onChange: (value: string) => void; options: string[]; value: string }): JSX.Element {
  return (
    <div className="flex flex-col gap-1.5">
      <Label>{label}</Label>
      <Select onValueChange={onChange} value={value}>
        <SelectTrigger>
          <SelectValue placeholder="-" />
        </SelectTrigger>
        <SelectContent>
          <SelectGroup>
            {options.map((option) => (
              <SelectItem key={option || "none"} value={option}>
                {option || "-"}
              </SelectItem>
            ))}
          </SelectGroup>
        </SelectContent>
      </Select>
    </div>
  );
}

function ScrollableSelectField({ label, loading = false, onChange, onOpen, options, value }: { label: string; loading?: boolean; onChange: (value: string) => void; onOpen?: () => void; options: string[]; value: string }): JSX.Element {
  const id = `scroll-select-${label.toLowerCase().replaceAll(/\s+/g, "-")}`;
  const [open, setOpen] = useState(false);
  return (
    <div className="flex min-w-0 flex-col gap-1.5">
      <Label htmlFor={id}>{label}</Label>
      <Popover
        open={open}
        onOpenChange={(nextOpen) => {
          setOpen(nextOpen);
          if (nextOpen) {
            onOpen?.();
          }
        }}
      >
        <PopoverTrigger asChild>
          <Button aria-expanded={open} className="h-9 w-full justify-between px-3 font-normal" id={id} role="combobox" type="button" variant="outline">
            <span className="min-w-0 truncate text-left">{value || "-"}</span>
            <span className="text-xs text-muted-foreground">Select</span>
          </Button>
        </PopoverTrigger>
        <PopoverContent align="start" className="w-[var(--radix-popover-trigger-width)] p-0">
          <div
            className="max-h-72 overflow-y-auto overscroll-contain p-1"
            data-slot="voice-options-scroll"
            onWheelCapture={(event) => {
              event.currentTarget.scrollTop += event.deltaY;
              event.stopPropagation();
            }}
          >
            {options.length === 0 ? (
              <div className="px-2 py-6 text-center text-sm text-muted-foreground">{loading ? "Loading" : "No options"}</div>
            ) : (
              options.map((option) => (
                <button
                  aria-selected={option === value}
                  className={cn("flex w-full items-center rounded-sm px-2 py-1.5 text-left text-sm hover:bg-accent hover:text-accent-foreground", option === value && "bg-accent text-accent-foreground")}
                  key={option || "none"}
                  onClick={() => {
                    onChange(option);
                    setOpen(false);
                  }}
                  role="option"
                  title={option}
                  type="button"
                >
                  <span className="min-w-0 truncate">{option || "-"}</span>
                </button>
              ))
            )}
          </div>
        </PopoverContent>
      </Popover>
    </div>
  );
}

function Toggle({ checked, label, onChange }: { checked: boolean; label: string; onChange: (checked: boolean) => void }): JSX.Element {
  return (
    <label className="flex h-9 items-center gap-2 rounded-md border px-3 text-sm">
      <input checked={checked} onChange={(event) => onChange(event.target.checked)} type="checkbox" />
      {label}
    </label>
  );
}

function SwitchField({ checked, label, onChange }: { checked: boolean; label: string; onChange: (checked: boolean) => void }): JSX.Element {
  const id = `switch-${label.toLowerCase().replaceAll(/\s+/g, "-")}`;
  return (
    <div className="flex flex-col gap-1.5">
      <Label htmlFor={id}>{label}</Label>
      <div className="flex h-9 items-center justify-between gap-3 rounded-md border px-3">
        <span className="text-sm text-muted-foreground">{checked ? "Enabled" : "Disabled"}</span>
        <Switch checked={checked} id={id} onCheckedChange={onChange} />
      </div>
    </div>
  );
}

function LoadingGrid(): JSX.Element {
  return (
    <div className="max-w-6xl">
      <Skeleton className="h-96 w-full" />
    </div>
  );
}

async function listModels(): Promise<ModelSpec[]> {
  const response = await listModelsPage("");
  return response.items ?? [];
}

function listModelsPage(cursor: string): Promise<PageResponse<ModelSpec>> {
  return expectData(listPeerModels({ query: pageQuery(cursor) })) as Promise<PageResponse<ModelSpec>>;
}

function listVoicesPage(cursor: string): Promise<PageResponse<Voice>> {
  return expectData(listClientVoices({ query: pageQuery(cursor) })) as Promise<PageResponse<Voice>>;
}

function listPetsPage(cursor: string): Promise<PageResponse<PetResource>> {
  return expectData(listPeerPets({ query: pageQuery(cursor) })) as Promise<PageResponse<PetResource>>;
}

function adoptPet(name: string): Promise<PetResource> {
  return expectData(adoptPeerPet({ body: { name } })) as Promise<PetResource>;
}

function updatePet(id: string, name: string): Promise<PetResource> {
  return expectData(putPeerPet({ body: { id, name }, path: { id } })) as Promise<PetResource>;
}

function deletePet(id: string): Promise<PetResource> {
  return expectData(deletePeerPet({ path: { id } })) as Promise<PetResource>;
}

function runPetAction(id: string, action: "feed" | "play" | "wash", prompt: string): Promise<PetResource> {
  const options = { body: { pet_id: id, prompt }, path: { id } };
  switch (action) {
    case "feed":
      return expectData(feedPeerPet(options)) as Promise<PetResource>;
    case "play":
      return expectData(playWithPeerPet(options)) as Promise<PetResource>;
    case "wash":
      return expectData(washPeerPet(options)) as Promise<PetResource>;
  }
}

function listRewardsPage(cursor: string): Promise<PageResponse<RewardResource>> {
  return expectData(listPeerRewards({ query: pageQuery(cursor) })) as Promise<PageResponse<RewardResource>>;
}

function getReward(id: string): Promise<RewardResource> {
  return expectData(getPeerReward({ path: { id } })) as Promise<RewardResource>;
}

function claimReward(prompt: string): Promise<RewardResource> {
  return expectData(claimPeerReward({ body: { prompt } })) as Promise<RewardResource>;
}

function listWalletTransactionsPage(cursor: string): Promise<PageResponse<WalletTransactionResource>> {
  return expectData(listPeerWalletTransactions({ query: pageQuery(cursor) })) as Promise<PageResponse<WalletTransactionResource>>;
}

function getWalletTransaction(id: string): Promise<WalletTransactionResource> {
  return expectData(getPeerWalletTransaction({ path: { id } })) as Promise<WalletTransactionResource>;
}

async function streamPlayableVoices(onVoice: (voice: Voice) => void): Promise<void> {
  const result = await streamPlayableVoicesSDK({ query: { limit: 100, provider_kind: "volc-tenant" }, sseMaxRetryAttempts: 0 });
  for await (const payload of result.stream as AsyncIterable<PlayVoiceStreamEvent>) {
    if (payload.error != null && payload.error !== "") {
      throw new Error(payload.error);
    }
    if (payload.voice != null) {
      onVoice(payload.voice as Voice);
    }
    if (payload.done === true) {
      break;
    }
  }
}

function mergeVoices(voices: Voice[]): Voice[] {
  const seen = new Set<string>();
  const out: Voice[] = [];
  for (const voice of voices) {
    if (seen.has(voice.id)) {
      continue;
    }
    seen.add(voice.id);
    out.push(voice);
  }
  return out;
}

function isPlayableVoice(voice: Voice): boolean {
  return voice.provider.kind === "volc-tenant";
}

async function listPeerResourcePage(name: string, cursor: string): Promise<PageResponse<ResourceItem>> {
  const query = pageQuery(cursor);
  switch (name) {
    case "credentials":
      return expectData(listPeerCredentials({ query })) as Promise<PageResponse<ResourceItem>>;
    case "friend-groups":
      return expectData(listPeerFriendGroups({ query })) as Promise<PageResponse<ResourceItem>>;
    case "friends":
      return expectData(listPeerFriends({ query })) as Promise<PageResponse<ResourceItem>>;
    case "models":
      return expectData(listPeerModels({ query })) as Promise<PageResponse<ResourceItem>>;
    case "voices":
      return expectData(listPeerVoices({ query })) as Promise<PageResponse<ResourceItem>>;
    case "workflows":
      return expectData(listPeerWorkflows({ query })) as Promise<PageResponse<ResourceItem>>;
    case "workspaces":
      return expectData(listPeerWorkspaces({ query })) as Promise<PageResponse<ResourceItem>>;
    default:
      throw new Error(`Unsupported peer resource: ${name}`);
  }
}

function getActiveWorkspace(): Promise<ActiveWorkspaceState> {
  return fetchJSON<ActiveWorkspaceState>("/peer-run/workspace");
}

function setActiveWorkspace(workspaceName: string): Promise<ActiveWorkspaceState> {
  return fetchJSON<ActiveWorkspaceState>("/peer-run/workspace", {
    body: JSON.stringify({ workspace_name: workspaceName }),
    headers: { "Content-Type": "application/json" },
    method: "PUT",
  });
}

function reloadActiveWorkspace(): Promise<ActiveWorkspaceState> {
  return fetchJSON<ActiveWorkspaceState>("/peer-run/workspace/reload", {
    method: "POST",
  });
}

function setActiveWorkspaceMode(mode: WorkspaceChatMode, workspaceName?: string): Promise<ActiveWorkspaceState> {
  return fetchJSON<ActiveWorkspaceState>("/peer-run/workspace/mode", {
    body: JSON.stringify({ mode, workspace_name: workspaceName }),
    headers: { "Content-Type": "application/json" },
    method: "PUT",
  });
}

function getWorkspaceDetails(workspaceName?: string): Promise<WorkspaceDetails> {
  const query = workspaceName == null || workspaceName.trim() === "" ? "" : `?workspace_name=${encodeURIComponent(workspaceName.trim())}`;
  return fetchJSON<WorkspaceDetails>(`/peer-run/workspace/details${query}`);
}

function updateWorkspaceDetails(request: { parameters: Record<string, unknown>; workspace_name: string; workflow_name: string }): Promise<WorkspaceDetails> {
  return fetchJSON<WorkspaceDetails>("/peer-run/workspace/details", {
    body: JSON.stringify(request),
    headers: { "Content-Type": "application/json" },
    method: "PUT",
  });
}

function getActiveWorkspaceHistory(): Promise<PageResponse<WorkspaceHistoryEntry>> {
  return fetchJSON<PageResponse<WorkspaceHistoryEntry>>("/peer-run/workspace/history");
}

function playActiveWorkspaceHistory(historyID: string): Promise<{ accepted: boolean; state?: string }> {
  return fetchJSON<{ accepted: boolean; state?: string }>("/peer-run/workspace/history/play", {
    body: JSON.stringify({ history_id: historyID }),
    headers: { "Content-Type": "application/json" },
    method: "POST",
  });
}

function getActiveWorkspaceMemoryStats(): Promise<WorkspaceMemoryStats> {
  return fetchJSON<WorkspaceMemoryStats>("/peer-run/workspace/memory/stats");
}

function recallActiveWorkspace(query: string): Promise<WorkspaceRecallResponse> {
  return fetchJSON<WorkspaceRecallResponse>("/peer-run/workspace/recall", {
    body: JSON.stringify({ limit: 10, query }),
    headers: { "Content-Type": "application/json" },
    method: "POST",
  });
}

async function createWorkspaceVoiceSession({
  onEvent,
  onRemoteStream,
  onState,
}: {
  onEvent: (event: PeerStreamEvent) => void;
  onRemoteStream: (stream: MediaStream) => void;
  onState: (state: RTCPeerConnectionState) => void;
}): Promise<WorkspaceVoiceSession> {
  const pc = new RTCPeerConnection();
  const eventChannel = pc.createDataChannel("event", { ordered: true });
  const remote = new MediaStream();
  const audioTransceiver = pc.addTransceiver("audio", { direction: "sendrecv" });
  let inputStream: MediaStream | null = null;
  let inputTrack: MediaStreamTrack | null = null;
  let audioEOSSent = false;
  let audioBOSSent = false;
  let inputStreamID = "";

  try {
    pc.onconnectionstatechange = () => {
      onState(pc.connectionState);
    };
    pc.ontrack = (event) => {
      remote.addTrack(event.track);
      onRemoteStream(remote);
    };
    eventChannel.onmessage = (message) => {
      const event = parsePeerStreamEvent(message.data);
      if (event != null) {
        onEvent(event);
      }
    };

    const offer = await pc.createOffer();
    await pc.setLocalDescription(offer);
    await waitForICEGatheringComplete(pc);
    const local = pc.localDescription;
    if (local == null) {
      throw new Error("WebRTC offer was not created.");
    }
    const answer = await createWebRTCOffer({ sdp: local.sdp, type: local.type });
    await pc.setRemoteDescription(answer);
    await waitForDataChannelOpen(eventChannel);
  } catch (err) {
    pc.close();
    throw err;
  }

  const sendEvent = (event: PeerStreamEvent): void => {
    if (eventChannel.readyState !== "open") {
      return;
    }
    eventChannel.send(JSON.stringify(event));
  };
  const ensureInputCapture = async (): Promise<MediaStreamTrack> => {
    if (inputTrack != null && inputTrack.readyState === "live") {
      return inputTrack;
    }
    if (!window.isSecureContext && window.location.hostname !== "127.0.0.1" && window.location.hostname !== "localhost") {
      throw new Error("Microphone capture requires a secure context.");
    }
    const media = await navigator.mediaDevices.getUserMedia({
      audio: {
        channelCount: 1,
        echoCancellation: true,
        noiseSuppression: true,
      },
    });
    const [track] = media.getAudioTracks();
    if (track == null) {
      for (const item of media.getTracks()) {
        item.stop();
      }
      throw new Error("Microphone capture returned no audio track.");
    }
    track.enabled = false;
    inputStream = media;
    inputTrack = track;
    try {
      await audioTransceiver.sender.replaceTrack(track);
    } catch (err) {
      stopInputCapture();
      throw err;
    }
    return track;
  };
  const stopInputCapture = (): void => {
    if (inputTrack != null) {
      inputTrack.stop();
      inputTrack = null;
    }
    if (inputStream != null) {
      for (const track of inputStream.getTracks()) {
        track.stop();
      }
      inputStream = null;
    }
  };

  const session: WorkspaceVoiceSession = {
    close: (reason?: string) => {
      void session.finishInputTurn(reason);
      stopInputCapture();
      for (const track of remote.getTracks()) {
        track.stop();
      }
      if (eventChannel.readyState === "open" || eventChannel.readyState === "connecting") {
        eventChannel.close();
      }
      pc.close();
    },
    finishInputTurn: async (error?: string) => {
      if (audioEOSSent || !audioBOSSent) {
        return;
      }
      const streamID = inputStreamID === "" ? newWorkspaceAudioStreamID() : inputStreamID;
      audioEOSSent = true;
      audioBOSSent = false;
      inputStreamID = "";
      sendEvent({
        ...(error != null && error !== "" ? { error } : {}),
        kind: "audio",
        mime_type: "audio/opus",
        stream_id: streamID,
        type: "eos",
        v: 1,
      });
      if (inputTrack != null) {
        inputTrack.enabled = false;
      }
    },
    startInputTurn: async (streamID: string) => {
      if (audioBOSSent) {
        return;
      }
      const track = await ensureInputCapture();
      inputStreamID = streamID;
      audioBOSSent = true;
      audioEOSSent = false;
      sendEvent({ kind: "audio", mime_type: "audio/opus", stream_id: inputStreamID, type: "bos", v: 1 });
      track.enabled = true;
    },
  };
  return session;
}

function newWorkspaceAudioStreamID(): string {
  return `audio-${Date.now().toString(36)}-${Math.random().toString(36).slice(2, 8)}`;
}

function parsePeerStreamEvent(data: unknown): PeerStreamEvent | null {
  try {
    const text = typeof data === "string" ? data : data instanceof ArrayBuffer ? new TextDecoder().decode(data) : "";
    if (text === "") {
      return null;
    }
    const parsed = JSON.parse(text) as Partial<PeerStreamEvent>;
    if (parsed.type == null) {
      return null;
    }
    return parsed as PeerStreamEvent;
  } catch {
    return null;
  }
}

function waitForICEGatheringComplete(pc: RTCPeerConnection): Promise<void> {
  if (pc.iceGatheringState === "complete") {
    return Promise.resolve();
  }
  return new Promise((resolve) => {
    const onStateChange = (): void => {
      if (pc.iceGatheringState === "complete") {
        pc.removeEventListener("icegatheringstatechange", onStateChange);
        resolve();
      }
    };
    pc.addEventListener("icegatheringstatechange", onStateChange);
  });
}

function waitForDataChannelOpen(channel: RTCDataChannel): Promise<void> {
  if (channel.readyState === "open") {
    return Promise.resolve();
  }
  return new Promise((resolve, reject) => {
    const cleanup = (): void => {
      channel.removeEventListener("open", onOpen);
      channel.removeEventListener("close", onClose);
      channel.removeEventListener("error", onError);
    };
    const onOpen = (): void => {
      cleanup();
      resolve();
    };
    const onClose = (): void => {
      cleanup();
      reject(new Error("WebRTC event channel closed before opening."));
    };
    const onError = (): void => {
      cleanup();
      reject(new Error("WebRTC event channel failed."));
    };
    channel.addEventListener("open", onOpen);
    channel.addEventListener("close", onClose);
    channel.addEventListener("error", onError);
  });
}

function createWebRTCOffer(offer: WebRTCSessionDescription): Promise<WebRTCSessionDescription> {
  return fetchJSON<WebRTCSessionDescription>("/webrtc/offer", {
    body: JSON.stringify(offer),
    headers: { "Content-Type": "application/json" },
    method: "POST",
  });
}

async function fetchJSON<T>(path: string, init?: RequestInit): Promise<T> {
  const response = await fetch(path, init);
  if (!response.ok) {
    throw new Error(await responseErrorMessage(response));
  }
  return (await response.json()) as T;
}

function normalizeWorkspaceState(state: ActiveWorkspaceState): ActiveWorkspaceState {
  return {
    ...state,
    runtime_state: state.runtime_state ?? (state.workspace_name == null || state.workspace_name === "" ? "no active workspace" : "unknown"),
    workspace_mode: normalizeWorkspaceMode(state.workspace_mode),
  };
}

function normalizeWorkspaceMode(mode: unknown): WorkspaceChatMode {
  switch (String(mode ?? "").trim().toLowerCase()) {
    case "realtime":
    case "real_time":
    case "real-time":
      return "realtime";
    default:
      return "push";
  }
}

function formatWorkspaceParameters(parameters: unknown): string {
  const value = parameters == null ? {} : parameters;
  return JSON.stringify(value, null, 2);
}

function parseWorkspaceParameters(text: string): Record<string, unknown> {
  const trimmed = text.trim();
  const parsed = trimmed === "" ? {} : JSON.parse(trimmed);
  if (parsed == null || typeof parsed !== "object" || Array.isArray(parsed)) {
    throw new Error("Workspace parameters must be a JSON object.");
  }
  return parsed as Record<string, unknown>;
}

function workspaceFeatureMessage(err: unknown): string {
  const message = toMessage(err).replace(/^HTTP 501 Not Implemented\s*/i, "").trim();
  return message === "" ? "This workspace feature is unavailable for the active agent." : message;
}

function getWallet(): Promise<WalletResource> {
  return expectData(getPeerWallet()) as Promise<WalletResource>;
}

function pageQuery(cursor: string): { cursor?: string; limit: number } {
  return cursor === "" ? { limit: 50 } : { cursor, limit: 50 };
}

function sectionTitle(section: Section): string {
  return sections.find((item) => item.id === section)?.label ?? "OpenAI Gateway";
}

function objectField(item: ResourceItem, key: string): ResourceItem {
  const value = item[key];
  return typeof value === "object" && value !== null && !Array.isArray(value) ? (value as ResourceItem) : {};
}

function stringField(item: ResourceItem, key: string): string {
  const value = item[key];
  if (value == null) {
    return "";
  }
  if (typeof value === "string") {
    return value;
  }
  if (typeof value === "number" || typeof value === "boolean") {
    return String(value);
  }
  return jsonSummary(value);
}

function sortWorkspacesByActivity(items: ResourceItem[]): ResourceItem[] {
  return [...items].sort((left, right) => {
    const leftTime = workspaceActivityTime(left);
    const rightTime = workspaceActivityTime(right);
    if (leftTime !== rightTime) {
      return rightTime - leftTime;
    }
    return stringField(left, "name").localeCompare(stringField(right, "name"));
  });
}

function workspaceActivityTime(item: ResourceItem): number {
  for (const key of ["last_active_at", "updated_at", "created_at"]) {
    const value = stringField(item, key);
    if (value === "") {
      continue;
    }
    const timestamp = Date.parse(value);
    if (!Number.isNaN(timestamp)) {
      return timestamp;
    }
  }
  return 0;
}

function summaryField(item: ResourceItem, key: string): string {
  const value = item[key];
  return value == null ? "" : jsonSummary(value);
}

function jsonSummary(value: unknown): string {
  if (typeof value === "string") {
    return value;
  }
  const text = JSON.stringify(value);
  if (text == null) {
    return "";
  }
  return text.length > 96 ? `${text.slice(0, 93)}...` : text;
}

function loadChatSessions(): ChatSession[] {
  try {
    const raw = localStorage.getItem(chatSessionsKey);
    if (raw != null) {
      const parsed = JSON.parse(raw) as ChatSession[];
      if (Array.isArray(parsed) && parsed.length > 0) {
        return parsed;
      }
    }
  } catch {
    // Ignore malformed local chat metadata.
  }
  return [createChatSession()];
}

function saveChatSessions(sessions: ChatSession[]): void {
  localStorage.setItem(chatSessionsKey, JSON.stringify(sessions));
}

function createChatSession(): ChatSession {
  const now = Date.now();
  return {
    createdAt: now,
    id: `chat-${now}-${Math.random().toString(36).slice(2, 8)}`,
    title: "Chat",
    updatedAt: now,
  };
}

function chatHistoryKey(sessionID: string): string {
  return `gizclaw.openai.chat.history.${sessionID}`;
}

function createThreadHistoryAdapter(sessionID: string, touchSession: (sessionID: string, firstUserText?: string) => void): ThreadHistoryAdapter {
  return {
    async load() {
      return loadThreadHistory(sessionID);
    },
    async append(item) {
      upsertThreadHistoryItem(sessionID, item);
      if (item.message.role === "user") {
        touchSession(sessionID, threadMessageText(item.message));
      } else {
        touchSession(sessionID);
      }
    },
  };
}

function loadThreadHistory(sessionID: string): ExportedMessageRepository {
  try {
    const raw = localStorage.getItem(chatHistoryKey(sessionID));
    if (raw == null) {
      return { headId: null, messages: [] };
    }
    const stored = JSON.parse(raw) as StoredHistory;
    return {
      headId: stored.headId ?? null,
      messages: (stored.messages ?? []).map((item) => ({
        ...item,
        message: {
          ...item.message,
          createdAt: new Date(item.message.createdAt),
        } as ThreadMessage,
      })),
    };
  } catch {
    return { headId: null, messages: [] };
  }
}

function saveThreadHistory(sessionID: string, repository: ExportedMessageRepository): void {
  const stored: StoredHistory = {
    headId: repository.headId ?? null,
    messages: repository.messages.map((item) => ({
      ...item,
      message: {
        ...item.message,
        createdAt: normalizeDate(item.message.createdAt).toISOString(),
      },
    })),
  };
  localStorage.setItem(chatHistoryKey(sessionID), JSON.stringify(stored));
}

function upsertThreadHistoryItem(sessionID: string, item: ExportedMessageRepositoryItem, localMessageID?: string): void {
  const repository = loadThreadHistory(sessionID);
  const index = repository.messages.findIndex((entry) => entry.message.id === item.message.id || (localMessageID != null && entry.message.id === localMessageID));
  const nextItem = { ...item, message: { ...item.message, createdAt: normalizeDate(item.message.createdAt) } };
  const messages = [...repository.messages];
  if (index >= 0) {
    messages[index] = nextItem;
  } else {
    messages.push(nextItem);
  }
  saveThreadHistory(sessionID, { headId: item.message.id, messages });
}

function normalizeDate(value: Date | string): Date {
  return value instanceof Date ? value : new Date(value);
}

function createOpenAIChatAdapter({
  model,
  onChatError,
  onCompleteText,
  sessionID,
  setSessionTitle,
  systemPrompt,
  temperature,
  thinking,
}: {
  model: string;
  onChatError: (message: string) => void;
  onCompleteText?: (text: string) => void;
  sessionID: string;
  setSessionTitle: (sessionID: string, title: string) => void;
  systemPrompt: string;
  temperature?: number;
  thinking?: ChatThinkingOptions;
}): ChatModelAdapter {
  return {
    async *run({ abortSignal, messages }): AsyncGenerator<ChatModelRunResult, void> {
      onChatError("");
      const chatMessages = toChatCompletionMessages(messages, systemPrompt);
      const shouldGenerateTitle = chatMessages.filter((message) => message.role === "user").length === 1;
      const body = {
        messages: chatMessages,
        model,
        stream: true,
        ...(Number.isFinite(temperature) ? { temperature } : {}),
        ...(thinking == null ? {} : { thinking }),
      } satisfies OpenAI.Chat.Completions.ChatCompletionCreateParamsStreaming & { thinking?: ChatThinkingOptions };
      let stream: AsyncIterable<OpenAI.Chat.Completions.ChatCompletionChunk>;
      try {
        stream = await getOpenAIClient().chat.completions.create(body, { signal: abortSignal });
      } catch (err) {
        if (isAbortError(err)) {
          return;
        }
        const errorText = chatRequestErrorText(model, errorToMessage(err));
        onChatError(errorText);
        yield chatErrorResult(errorText);
        return;
      }

      if (shouldGenerateTitle) {
        void generateChatTitle(model, chatMessages, abortSignal, Number.isFinite(temperature) ? 0.2 : undefined)
          .then((title) => {
            if (title !== "") {
              setSessionTitle(sessionID, title);
            }
          })
          .catch(() => {
            // Keep the default title if title generation fails.
          });
      }

      let text = "";
      try {
        for await (const chunk of stream) {
          const delta = chunk.choices[0]?.delta?.content ?? "";
          if (delta !== "") {
            text += delta;
            yield { content: [{ type: "text", text }] };
          }
        }
      } catch (err) {
        if (isAbortError(err)) {
          return;
        }
        const errorText = chatRequestErrorText(model, errorToMessage(err));
        onChatError(errorText);
        yield chatErrorResult(errorText, text);
        return;
      }
      onCompleteText?.(text);
      yield { content: [{ type: "text", text }], status: { type: "complete", reason: "stop" } };
    },
  };
}

function isTransientSpeechProxyError(message: string): boolean {
  return message.includes("kcp: conn closed: local") || message.includes("gizhttp: read response: kcp: timeout");
}

function chatErrorResult(errorText: string, partialText = ""): ChatModelRunResult {
  const text = partialText === "" ? errorText : `${partialText}\n\n${errorText}`;
  return {
    content: [{ type: "text", text }],
    status: { type: "incomplete", reason: "error", error: errorText },
  };
}

function chatRequestErrorText(model: string, detail: string): string {
  const trimmed = detail.trim();
  const message = trimmed === "" ? "No error detail was returned by the gateway or upstream provider. Check the server logs for this request." : trimmed;
  return `Chat request failed for ${model}.\n\n${message}`;
}

function openAIErrorPayloadMessage(payload: unknown): string {
  if (typeof payload !== "object" || payload == null) {
    return typeof payload === "string" ? payload : "";
  }
  const record = payload as Record<string, unknown>;
  const error = record.error;
  if (typeof error === "string") {
    return error;
  }
  if (typeof error === "object" && error != null) {
    const errorRecord = error as Record<string, unknown>;
    const message = typeof errorRecord.message === "string" ? errorRecord.message : "";
    const code = typeof errorRecord.code === "string" ? errorRecord.code : "";
    const kind = typeof errorRecord.type === "string" ? errorRecord.type : "";
    const suffix = [code, kind].filter(Boolean).join(" / ");
    if (message !== "") {
      return suffix === "" ? message : `${message}\n${suffix}`;
    }
    return suffix === "" ? JSON.stringify(error) : suffix;
  }
  if (typeof record.message === "string") {
    return record.message;
  }
  return JSON.stringify(payload);
}

function errorToMessage(error: unknown): string {
  if (error instanceof Error) {
    return error.message;
  }
  if (typeof error === "string") {
    return error;
  }
  return JSON.stringify(error);
}

function isAbortError(error: unknown): boolean {
  return error instanceof DOMException && error.name === "AbortError";
}

async function generateChatTitle(model: string, messages: ChatCompletionMessageParam[], abortSignal: AbortSignal, temperature?: number): Promise<string> {
  const firstUserContent = messages.find((message) => message.role === "user")?.content;
  const firstUserMessage = typeof firstUserContent === "string" ? firstUserContent.trim() : "";
  if (firstUserMessage === "") {
    return "";
  }
  const response = await getOpenAIClient().chat.completions.create(
    {
      messages: [
        {
          role: "system",
          content: "Generate a concise chat title. Return only the title, no quotes, no punctuation suffix. Use the user's language. Keep it under 8 words.",
        },
        {
          role: "user",
          content: firstUserMessage,
        },
      ],
      model,
      ...(Number.isFinite(temperature) ? { temperature } : {}),
    },
    { signal: abortSignal },
  );
  return cleanChatTitle(response.choices[0]?.message?.content ?? "");
}

function cleanChatTitle(value: string): string {
  return value
    .trim()
    .replace(/^["'“”‘’]+|["'“”‘’]+$/g, "")
    .replace(/[。.!！?？]+$/g, "")
    .slice(0, 48);
}

function toChatCompletionMessages(messages: readonly ThreadMessage[], systemPrompt: string): ChatCompletionMessageParam[] {
  const result: ChatCompletionMessageParam[] = [];
  if (systemPrompt.trim() !== "") {
    result.push({ role: "system", content: systemPrompt.trim() });
  }
  for (const message of messages) {
    if (message.role !== "user" && message.role !== "assistant" && message.role !== "system") {
      continue;
    }
    const content = threadMessageText(message);
    if (content.trim() !== "") {
      result.push({ role: message.role, content });
    }
  }
  return result;
}

function threadMessageText(message: ThreadMessage): string {
  return message.content
    .map((part) => (part.type === "text" ? part.text : ""))
    .filter(Boolean)
    .join("\n");
}

function formatDate(value: number | string | undefined | null): string {
  if (value == null || value === "") {
    return "-";
  }
  const date = typeof value === "number" ? new Date(value < 10_000_000_000 ? value * 1000 : value) : new Date(value);
  if (Number.isNaN(date.getTime())) {
    return String(value);
  }
  return date.toLocaleString();
}

function formatDuration(value: number | undefined | null): string {
  if (value == null || !Number.isFinite(value)) {
    return "-";
  }
  if (value < 1000) {
    return `${Math.round(value)}ms`;
  }
  return `${(value / 1000).toFixed(1)}s`;
}

function formatBytes(value: number | undefined | null): string {
  if (value == null || !Number.isFinite(value)) {
    return "-";
  }
  if (value < 1024) {
    return `${Math.round(value)} B`;
  }
  if (value < 1024 * 1024) {
    return `${(value / 1024).toFixed(1)} KiB`;
  }
  return `${(value / 1024 / 1024).toFixed(1)} MiB`;
}

function formatScore(value: number | undefined | null): string {
  if (value == null || !Number.isFinite(value)) {
    return "-";
  }
  return value.toFixed(3);
}

function compactID(value: string): string {
  if (value.length <= 36) {
    return value;
  }
  return `${value.slice(0, 20)}...${value.slice(-8)}`;
}

const root = document.querySelector<HTMLElement>("#app");
if (root === null) {
  throw new Error("missing #app root");
}

createRoot(root).render(
  <StrictMode>
    <App />
  </StrictMode>,
);
