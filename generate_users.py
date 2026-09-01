import json
import random
import string
import sys
import os

def generate_random_string(length=8):
    chars = string.ascii_lowercase + string.digits
    return ''.join(random.choice(chars) for _ in range(length))

def generate_random_password(length=10):
    chars = string.ascii_letters + string.digits
    return ''.join(random.choice(chars) for _ in range(length))

def generate_users(count=5000, output_file="users_5000.json"):
    print(f"Gerando {count} usuários aleatórios...")
    users = []
    
    # Conjunto para garantir usernames únicos
    used_usernames = set()
    
    for i in range(1, count + 1):
        # Gerar username único
        while True:
            # Padrão: user_<numero>_<sufixo> (ex: user_0001_a9f2)
            suffix = generate_random_string(4)
            username = f"user_{i:04d}_{suffix}"
            if username not in used_usernames:
                used_usernames.add(username)
                break
        
        password = generate_random_password(10)
        validate_days = random.choice([15, 30, 45, 60, 90])
        
        user = {
            "username": username,
            "password": password,
            "limit": 0,
            "validate": validate_days,
            "is_test": False,
            "time": 0
        }
        users.append(user)
        
        if i % 1000 == 0:
            print(f"Progresso: {i}/{count} usuários gerados...")
            
    print(f"Salvando no arquivo '{output_file}'...")
    with open(output_file, "w", encoding="utf-8") as f:
        json.dump(users, f, indent=2, ensure_ascii=False)
        
    file_size_mb = os.path.getsize(output_file) / (1024 * 1024)
    print(f"[OK] Arquivo '{output_file}' gerado com sucesso!")
    print(f"Total de usuarios: {len(users)} | Tamanho do arquivo: {file_size_mb:.2f} MB")

if __name__ == "__main__":
    count = 5000
    if len(sys.argv) > 1:
        try:
            count = int(sys.argv[1])
        except ValueError:
            print("Uso: python generate_users.py [quantidade]")
            sys.exit(1)
            
    output_filename = f"users_{count}.json"
    generate_users(count, output_filename)
