const $ = (selector) => document.querySelector(selector);
let uploadedVideoID = null, room = null, socket = null, applyingRemoteState = false, currentUser = null;
let reconnectTimer = null, reconnectAttempts = 0, intentionalDisconnect = false;
function notice(text) { $("#notice").textContent = text; }
async function api(path, options = {}) {
  const response = await fetch(path, { credentials: "same-origin", ...options });
  const body = response.status === 204 ? null : await response.json().catch(() => ({}));
  if (!response.ok) throw new Error(body.message || "Request failed");
  return body;
}
function json(method, value) { return { method, headers: {"Content-Type": "application/json"}, body: JSON.stringify(value) }; }
function showStudio() { $("#auth-panel").classList.add("hidden"); $("#studio").classList.remove("hidden"); }
async function loadStudio() {
  currentUser = await api("/api/me"); showStudio(); $("#profile").textContent = `Signed in as ${currentUser.username} (${currentUser.email})`;
  const [videos, rooms] = await Promise.all([api("/api/videos"), api("/api/rooms")]);
  $("#video-list").replaceChildren(...videos.map(v => { const p = document.createElement("p"); p.textContent = `${v.name} (${Math.round(v.sizeBytes / 1048576 * 10) / 10} MB)`; return p; }));
  $("#room-list").replaceChildren(...rooms.map(r => { const button = document.createElement("button"); button.className = "secondary"; button.textContent = r.title; button.onclick = () => openRoom(r); return button; }));
}
$("#auth-form").addEventListener("submit", async (event) => { event.preventDefault(); const action = event.submitter.value; try { const payload = {email: $("#email").value, password: $("#password").value}; if (action === "register") payload.username = $("#username").value; await api(`/api/auth/${action}`, json("POST", payload)); if (action === "register") { $("#verify-form").classList.remove("hidden"); notice("Check your email for the 6-digit verification code."); } else { await loadStudio(); notice("You are signed in."); } } catch (error) { notice(error.message); }});
$("#verify-form").addEventListener("submit", async (event) => { event.preventDefault(); try { await api("/api/auth/verify", json("POST", {email:$("#email").value, code:$("#otp-code").value})); await loadStudio(); notice("Account verified and signed in."); } catch (error) { notice(error.message); }});
$("#logout").addEventListener("click", async () => { await api("/api/auth/logout", {method:"POST"}); intentionalDisconnect = true; room = null; socket?.close(); $("#studio").classList.add("hidden"); $("#room").classList.add("hidden"); $("#auth-panel").classList.remove("hidden"); notice("Signed out."); });
$("#upload-form").addEventListener("submit", async (event) => { event.preventDefault(); const file = $("#video-file").files[0]; if (!file) return; const data = new FormData(); data.append("video", file); try { const video = await api("/api/videos", {method:"POST", body:data}); uploadedVideoID = video.id; $("#room-form").classList.remove("hidden"); await loadStudio(); notice(`${video.name} uploaded. Now create your room.`); } catch (error) { notice(error.message); }});
$("#room-form").addEventListener("submit", async (event) => { event.preventDefault(); try { openRoom(await api("/api/rooms", json("POST", {videoId:uploadedVideoID, title:$("#room-title").value}))); } catch (error) { notice(error.message); }});
$("#join-form").addEventListener("submit", async (event) => { event.preventDefault(); try { openRoom(await api(`/api/invites/${encodeURIComponent($("#invite-token").value.trim())}/join`, {method:"POST"})); } catch (error) { notice(error.message); }});
function appendChat(message) { const line = document.createElement("p"); line.className = "message"; line.textContent = `${message.senderName}: ${message.text}`; $("#chat-log").append(line); $("#chat-log").scrollTop = $("#chat-log").scrollHeight; }
async function openRoom(nextRoom) {
  intentionalDisconnect = true;
  if (reconnectTimer) { clearTimeout(reconnectTimer); reconnectTimer = null; }
  if (socket) socket.close();
  room = nextRoom; intentionalDisconnect = false; reconnectAttempts = 0;
  $("#studio").classList.add("hidden"); $("#room").classList.remove("hidden"); $("#room-title-display").textContent = room.title; $("#invite-display").textContent = room.inviteToken; $("#chat-log").replaceChildren(); $("#participants").replaceChildren();
  const history = await api(`/api/rooms/${encodeURIComponent(room.id)}/chat`); history.forEach(appendChat);
  const player = $("#player"); player.src = `/api/rooms/${encodeURIComponent(room.id)}/video`; player.currentTime = room.position || 0;
  connectSocket();
}
function connectSocket() {
  if (!room || intentionalDisconnect) return;
  const scheme = location.protocol === "https:" ? "wss" : "ws";
  socket = new WebSocket(`${scheme}://${location.host}/ws?roomId=${encodeURIComponent(room.id)}`);
  $("#connection").textContent = reconnectAttempts ? `Reconnecting (attempt ${reconnectAttempts})…` : "Connecting…";
  socket.onopen = () => { reconnectAttempts = 0; $("#connection").textContent = "Connected"; };
  socket.onclose = () => {
    $("#connection").textContent = "Disconnected";
    if (!intentionalDisconnect && room) {
      reconnectAttempts++;
      const delay = Math.min(10000, 500 * 2 ** Math.min(reconnectAttempts - 1, 4));
      reconnectTimer = setTimeout(connectSocket, delay);
    }
  };
  socket.onerror = () => notice("Live connection interrupted; reconnecting…");
  socket.onmessage = ({data}) => { try { handleMessage(JSON.parse(data)); } catch { notice("Received an invalid room message."); } };
}
function send(type, extra = {}) { if (socket?.readyState === WebSocket.OPEN) socket.send(JSON.stringify({type, ...extra})); }
function handleMessage(message) {
  if (message.type === "room.state") {
    const player = $("#player"); const drift = message.position - player.currentTime; applyingRemoteState = true;
    if (Math.abs(drift) > .5) player.currentTime = message.position;
    message.playing ? player.play().catch(() => {}) : player.pause();
    if (message.locked != null) { room.locked = message.locked; renderRoomControls(); }
    if (message.roomId === room.id && message.hostId) { room.hostId = message.hostId; renderRoomControls(); }
    setTimeout(() => applyingRemoteState = false, 0);
  }
  else if (message.type === "presence.ping") send("presence.pong", {pingSent:message.pingSent});
  else if (message.type === "chat.message") appendChat(message);
  else if (message.type === "presence.update") { $("#participants").replaceChildren(...(message.users || []).map(user => { const row = document.createElement("div"); row.textContent = `${user.username} (${user.pingMs >= 0 ? user.pingMs + " ms" : "connecting"})`; if (currentUser?.id === room.hostId && user.userId !== currentUser.id) { const kick = document.createElement("button"); kick.className = "secondary"; kick.textContent = "Kick"; kick.onclick = async () => { try { await api(`/api/rooms/${room.id}/members/${user.userId}/kick`, {method:"POST"}); } catch (e) { notice(e.message); } }; const transfer = document.createElement("button"); transfer.className = "secondary"; transfer.textContent = "Make host"; transfer.onclick = async () => { try { const updated = await api(`/api/rooms/${room.id}/host/${user.userId}`, {method:"POST"}); room.hostId = updated.hostId; renderRoomControls(); } catch (e) { notice(e.message); } }; row.append(" ", kick, " ", transfer); } return row; })); }
  else if (message.type === "error") {
    notice(message.text);
    if (message.code === "kicked") {
      intentionalDisconnect = true;
      socket?.close();
      $("#room").classList.add("hidden");
      $("#studio").classList.remove("hidden");
      loadStudio().catch(e => notice(e.message));
    }
    function renderRoomControls() {
      const button = $("#lock-room");
      if (!room || currentUser?.id !== room.hostId) { button.classList.add("hidden"); return; }
      button.classList.remove("hidden"); button.textContent = room.locked ? "Unlock room" : "Lock room";
    }
  }
}
$("#player").addEventListener("play", () => { if (!applyingRemoteState) send("playback.play", {position:$("#player").currentTime}); });
$("#player").addEventListener("pause", () => { if (!applyingRemoteState) send("playback.pause", {position:$("#player").currentTime}); });
$("#player").addEventListener("seeked", () => { if (!applyingRemoteState) send("playback.seek", {position:$("#player").currentTime}); });
$("#chat-form").addEventListener("submit", (event) => { event.preventDefault(); const input=$("#chat-text"); const text=input.value.trim(); if (text) send("chat.send", {text}); input.value=""; });
$("#leave").addEventListener("click", () => { intentionalDisconnect = true; if (reconnectTimer) clearTimeout(reconnectTimer); socket?.close(); $("#room").classList.add("hidden"); $("#studio").classList.remove("hidden"); loadStudio().catch(e => notice(e.message)); });
$("#lock-room").addEventListener("click", async () => { try { const updated = await api(`/api/rooms/${room.id}/lock`, json("POST", {locked:!room.locked})); room.locked = updated.locked; renderRoomControls(); } catch (e) { notice(e.message); } });
loadStudio().catch(() => {});
