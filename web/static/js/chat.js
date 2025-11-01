// 全局变量
let socket = null;
let currentUsername = '';
let isConnected = false;
let onlineUsers = new Set();

// DOM 元素
const loginScreen = document.getElementById('login-screen');
const chatScreen = document.getElementById('chat-screen');
const usernameInput = document.getElementById('username-input');
const messageInput = document.getElementById('message-input');
const messagesContainer = document.getElementById('messages-container');
const currentUsernameSpan = document.getElementById('current-username');
const statusIndicator = document.getElementById('status-indicator');
const userList = document.getElementById('user-list');
const userCount = document.getElementById('user-count');
const sendBtn = document.getElementById('send-btn');
const notification = document.getElementById('notification');

// 初始化
document.addEventListener('DOMContentLoaded', function() {
    // 聚焦用户名输入框
    usernameInput.focus();
    
    // 检查 WebSocket 支持
    if (!window.WebSocket) {
        showNotification('您的浏览器不支持 WebSocket', 'error');
        return;
    }
    
    // 禁用发送按钮
    updateSendButton();
});

// 加入聊天
function joinChat() {
    const username = usernameInput.value.trim();
    
    if (!username) {
        showNotification('请输入用户名', 'error');
        usernameInput.focus();
        return;
    }
    
    if (username.length > 20) {
        showNotification('用户名不能超过20个字符', 'error');
        usernameInput.focus();
        return;
    }
    
    // 检查用户名格式
    if (!/^[a-zA-Z0-9_\u4e00-\u9fa5]+$/.test(username)) {
        showNotification('用户名只能包含字母、数字、下划线和中文', 'error');
        usernameInput.focus();
        return;
    }
    
    currentUsername = username;
    currentUsernameSpan.textContent = username;
    
    // 切换到聊天界面
    loginScreen.style.display = 'none';
    chatScreen.style.display = 'flex';
    
    // 连接到服务器
    connectToServer();
    
    // 聚焦消息输入框
    setTimeout(() => {
        messageInput.focus();
    }, 100);
}

// 连接到服务器
function connectToServer() {
    try {
        updateStatus('connecting');
        
        // 注意：这里需要实现 WebSocket 到 gRPC 的桥接
        // 暂时使用 WebSocket 连接，实际项目中需要服务器端支持
        const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
        const wsUrl = `${protocol}//${window.location.host}/ws`;
        
        socket = new WebSocket(wsUrl);
        
        socket.onopen = function(event) {
            console.log('WebSocket 连接已建立');
            isConnected = true;
            updateStatus('connected');
            
            // 发送加入消息
            sendJoinMessage();
            
            showNotification('连接成功！', 'success');
        };
        
        socket.onmessage = function(event) {
            try {
                const message = JSON.parse(event.data);
                handleMessage(message);
            } catch (e) {
                console.error('解析消息失败:', e);
            }
        };
        
        socket.onclose = function(event) {
            console.log('WebSocket 连接已关闭', event);
            isConnected = false;
            updateStatus('disconnected');
            updateSendButton();
            
            if (event.code !== 1000) { // 非正常关闭
                showNotification('连接已断开，正在尝试重连...', 'error');
                // 自动重连
                setTimeout(connectToServer, 3000);
            }
        };
        
        socket.onerror = function(error) {
            console.error('WebSocket 错误:', error);
            updateStatus('disconnected');
            showNotification('连接失败，请检查网络连接', 'error');
        };
        
    } catch (error) {
        console.error('连接失败:', error);
        updateStatus('disconnected');
        showNotification('连接失败: ' + error.message, 'error');
    }
}

// 发送加入消息
function sendJoinMessage() {
    if (socket && socket.readyState === WebSocket.OPEN) {
        const joinMessage = {
            type: 'join',
            user: currentUsername,
            text: 'has joined',
            timestamp: new Date().toISOString()
        };
        
        socket.send(JSON.stringify(joinMessage));
    }
}

// 处理收到的消息
function handleMessage(message) {
    switch (message.type) {
        case 'chat':
            displayMessage(message);
            break;
        case 'system':
            displaySystemMessage(message.text);
            break;
        case 'userList':
            updateUserList(message.users);
            break;
        case 'userJoin':
            onlineUsers.add(message.user);
            updateUserCount();
            displaySystemMessage(`${message.user} 加入了聊天室`);
            break;
        case 'userLeave':
            onlineUsers.delete(message.user);
            updateUserCount();
            displaySystemMessage(`${message.user} 离开了聊天室`);
            break;
        case 'error':
            showNotification(message.text, 'error');
            break;
        default:
            console.log('未知消息类型:', message);
    }
}

// 显示消息
function displayMessage(message) {
    const messageDiv = document.createElement('div');
    messageDiv.className = 'message';
    
    // 判断消息类型
    if (message.user === currentUsername) {
        messageDiv.classList.add('sent');
    } else {
        messageDiv.classList.add('received');
    }
    
    // 私人消息样式
    if (message.recipientUser) {
        messageDiv.classList.add('private');
    }
    
    // 构建消息内容
    let messageContent = '';
    
    if (message.user !== currentUsername) {
        messageContent += `<div class="message-header">${escapeHtml(message.user)}</div>`;
    }
    
    messageContent += `<div class="message-text">${escapeHtml(message.text)}</div>`;
    
    if (message.recipientUser) {
        const recipientText = message.user === currentUsername 
            ? `发送给 ${message.recipientUser}` 
            : '私人消息';
        messageContent += `<div class="message-time">${recipientText}</div>`;
    }
    
    // 添加时间戳
    const time = new Date(message.timestamp || new Date()).toLocaleTimeString();
    messageContent += `<div class="message-time">${time}</div>`;
    
    messageDiv.innerHTML = messageContent;
    messagesContainer.appendChild(messageDiv);
    
    // 滚动到底部
    scrollToBottom();
}

// 显示系统消息
function displaySystemMessage(text) {
    const messageDiv = document.createElement('div');
    messageDiv.className = 'message system';
    messageDiv.innerHTML = `
        <div class="message-text">${escapeHtml(text)}</div>
        <div class="message-time">${new Date().toLocaleTimeString()}</div>
    `;
    
    messagesContainer.appendChild(messageDiv);
    scrollToBottom();
}

// 发送消息
function sendMessage() {
    const text = messageInput.value.trim();
    
    if (!text) {
        return;
    }
    
    if (!isConnected) {
        showNotification('未连接到服务器', 'error');
        return;
    }
    
    if (text.length > 500) {
        showNotification('消息长度不能超过500个字符', 'error');
        return;
    }
    
    let recipientUser = '';
    let messageText = text;
    
    // 处理私人消息
    if (text.startsWith('/pm ')) {
        const parts = text.split(' ');
        if (parts.length < 3) {
            showNotification('私人消息格式错误，请使用: /pm 用户名 消息', 'error');
            return;
        }
        
        recipientUser = parts[1];
        messageText = parts.slice(2).join(' ');
        
        if (recipientUser === currentUsername) {
            showNotification('不能给自己发送私人消息', 'error');
            return;
        }
        
        if (!onlineUsers.has(recipientUser)) {
            showNotification(`用户 ${recipientUser} 不在线`, 'error');
            return;
        }
    }
    
    // 构建消息对象
    const message = {
        type: 'chat',
        user: currentUsername,
        text: messageText,
        recipientUser: recipientUser,
        timestamp: new Date().toISOString()
    };
    
    // 发送消息
    try {
        socket.send(JSON.stringify(message));
        
        // 清空输入框
        messageInput.value = '';
        updateSendButton();
        
        // 注意：不在这里显示消息，等待服务器返回后再显示
        // 这样避免重复显示消息
        
    } catch (error) {
        console.error('发送消息失败:', error);
        showNotification('发送消息失败', 'error');
    }
}

// 断开连接
function disconnect() {
    if (socket) {
        socket.close(1000, '用户主动断开');
    }
    
    // 重置状态
    isConnected = false;
    currentUsername = '';
    onlineUsers.clear();
    
    // 清空消息
    messagesContainer.innerHTML = '';
    
    // 切换到登录界面
    chatScreen.style.display = 'none';
    loginScreen.style.display = 'flex';
    
    // 重置输入框
    usernameInput.value = '';
    messageInput.value = '';
    
    // 聚焦用户名输入框
    setTimeout(() => {
        usernameInput.focus();
    }, 100);
    
    showNotification('已断开连接', 'info');
}

// 更新用户列表
function updateUserList(users) {
    onlineUsers.clear();
    users.forEach(user => onlineUsers.add(user));
    
    userList.innerHTML = '';
    
    users.forEach(user => {
        const userItem = document.createElement('div');
        userItem.className = 'user-item';
        
        if (user === currentUsername) {
            userItem.classList.add('current-user');
        }
        
        userItem.innerHTML = `
            <i class="fas fa-user"></i>
            <span>${escapeHtml(user)}</span>
        `;
        
        // 点击用户名插入私聊命令
        if (user !== currentUsername) {
            userItem.style.cursor = 'pointer';
            userItem.onclick = () => {
                messageInput.value = `/pm ${user} `;
                messageInput.focus();
            };
        }
        
        userList.appendChild(userItem);
    });
    
    updateUserCount();
}

// 更新用户数量
function updateUserCount() {
    userCount.textContent = onlineUsers.size;
}

// 更新连接状态
function updateStatus(status) {
    statusIndicator.className = `status ${status}`;
    
    switch (status) {
        case 'connecting':
            statusIndicator.innerHTML = '<i class="fas fa-circle"></i> 连接中...';
            break;
        case 'connected':
            statusIndicator.innerHTML = '<i class="fas fa-circle"></i> 已连接';
            break;
        case 'disconnected':
            statusIndicator.innerHTML = '<i class="fas fa-circle"></i> 已断开';
            break;
    }
}

// 更新发送按钮状态
function updateSendButton() {
    const hasText = messageInput.value.trim().length > 0;
    sendBtn.disabled = !isConnected || !hasText;
}

// 处理键盘事件
function handleKeyPress(event) {
    if (event.key === 'Enter') {
        event.preventDefault();
        if (document.activeElement === usernameInput) {
            joinChat();
        } else if (document.activeElement === messageInput) {
            sendMessage();
        }
    }
}

// 监听消息输入框变化
messageInput.addEventListener('input', updateSendButton);

// 滚动到底部
function scrollToBottom() {
    setTimeout(() => {
        messagesContainer.scrollTop = messagesContainer.scrollHeight;
    }, 10);
}

// 显示通知
function showNotification(text, type = 'info') {
    const notificationText = document.getElementById('notification-text');
    notificationText.textContent = text;
    
    notification.className = `notification ${type}`;
    notification.style.display = 'block';
    
    // 自动隐藏通知
    setTimeout(hideNotification, 5000);
}

// 隐藏通知
function hideNotification() {
    notification.style.display = 'none';
}

// HTML 转义
function escapeHtml(text) {
    const div = document.createElement('div');
    div.textContent = text;
    return div.innerHTML;
}

// 窗口关闭前确认
window.addEventListener('beforeunload', function(event) {
    if (isConnected) {
        event.preventDefault();
        event.returnValue = '您确定要离开聊天室吗？';
        return event.returnValue;
    }
});

// 处理页面可见性变化
document.addEventListener('visibilitychange', function() {
    if (document.visibilityState === 'visible' && !isConnected && currentUsername) {
        // 页面重新变为可见且之前已连接，尝试重连
        setTimeout(connectToServer, 1000);
    }
});

// 开发模式：添加一些测试功能
if (window.location.hostname === 'localhost' || window.location.hostname === '127.0.0.1') {
    // 添加测试用户按钮（仅在开发模式）
    console.log('开发模式：可使用测试功能');
    
    // 模拟收到消息的函数（用于测试）
    window.simulateMessage = function(user, text, recipientUser = '') {
        const message = {
            type: 'chat',
            user: user,
            text: text,
            recipientUser: recipientUser,
            timestamp: new Date().toISOString()
        };
        handleMessage(message);
    };
    
    // 模拟系统消息
    window.simulateSystemMessage = function(text) {
        displaySystemMessage(text);
    };
    
    // 模拟用户列表更新
    window.simulateUserList = function(users) {
        updateUserList(users);
    };
}