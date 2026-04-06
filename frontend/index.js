const loginDiv = document.getElementById("login");
const roomsDiv = document.getElementById("rooms");
const roomList = document.getElementById("room-list");
const chatDiv = document.getElementById("chat");
const messages = document.getElementById("messages");
const input = document.getElementById("message");
const btn = document.getElementById("send");
const usernameEl = document.getElementById("username");
const passwordEl = document.getElementById("password");
const newRoomEl = document.getElementById("new-room");
const connectBtn = document.getElementById("connect");
const createRoomBtn = document.getElementById("create-room")
const leaveRoomBtn = document.getElementById("leave-room")

let token = null;
let ws = null;
let rooms = null;
let room_id = null;

connectBtn.addEventListener("click", function () {
  const username = usernameEl.value.trim();
  const password = passwordEl.value.trim();
  if (username === "" || username.length > 20) return;
  if (password === "" || password.length < 8) return;

  fetch("/login", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ username, password }),
  })
    .then(res => res.json())
    .then(data => {
      if (data.error) {
        appendMessage("Login failed: " + data.error);
        return;
      }
      token = data.token;
    }).then(async _ => await loadRooms());
});

createRoomBtn.addEventListener("click", async function () {
  const newRoom = newRoomEl.value.trim();
  if (newRoom === "" || newRoom.length > 50) return;

  fetch("/rooms", {
    method: "POST",
    headers: { "Content-Type": "application/json", "Authorization": "Bearer " + token },
    body: JSON.stringify({ name: newRoom }),
  })
    .then(res => res.json())
    .then(data => {
      if (data.error) {
        appendMessage("Create room failed: " + data.error);
        return;
      }
      loadRooms();
    });
});

btn.addEventListener("click", sendMessage);
leaveRoomBtn.addEventListener("click", leaveRoom)

function leaveRoom() {
  messages.replaceChildren();
  if (ws) {
    ws.close();
    ws = null;
  }
  room_id = null;
  loadRooms();
}

input.addEventListener("keydown", function (event) {
  if (event.key === "Enter") sendMessage();
});

async function loadRooms() {
  try {
    fetch("/rooms", {
      method: "GET",
      headers: {
        "Authorization": "Bearer " + token,
      }
    })
      .then(res => res.json())
      .then(res => {
        roomList.innerHTML = "";
        rooms = res
        rooms.forEach(room => {
          const li = document.createElement("li");
          const roomName = room.name
          const roomID = room.id
          li.textContent = roomName;
          li.addEventListener("click", function () {
            rooms.forEach(room => {
              if (room.name === roomName) {
                room_id = roomID
                connectWebSocket()
              }
            })
          })
          roomList.appendChild(li);
        })

        loginDiv.style.display = "none"
        roomsDiv.style.display = "block"
      });
  } catch (err) {
    appendMessage("Failed to load rooms")
  }
}

function sendMessage() {
  const text = input.value.trim();
  if (text === "" || !ws || ws.readyState !== WebSocket.OPEN) return;
  ws.send(JSON.stringify({ text: text }));
  input.value = "";
}

function appendMessage(text) {
  const div = document.createElement("div");
  div.textContent = text;
  messages.appendChild(div);
  messages.scrollTop = messages.scrollHeight;
}

function connectWebSocket() {
  messages.replaceChildren();
  ws = new WebSocket("ws://" + location.host + "/ws?token=" + token + "&room_id=" + room_id);

  ws.onopen = function () {
    loginDiv.style.display = "none";
    roomsDiv.style.display = "none"
    chatDiv.style.display = "block";
    appendMessage("-- connected --");
  };

  ws.onmessage = function (event) {
    const msg = JSON.parse(event.data);
    const time = new Date(msg.timestamp).toLocaleTimeString();
    appendMessage("[" + time + "] " + msg.username + ": " + msg.text);
  };

  ws.onclose = function () {
    appendMessage("-- disconnected --");
    loginDiv.style.display = "block";
    chatDiv.style.display = "none";
  };

  ws.onerror = function () {
    appendMessage("-- error --");
  };
}
