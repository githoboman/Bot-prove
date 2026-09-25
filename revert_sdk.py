import os

def revert_imports(root_dir):
    replacements = {
        "github.com/make-software/bot-go-sdk": "github.com/make-software/casper-go-sdk",
        "@make-software/botclick": "@make-software/csprclick",
        "botclick-core": "csprclick-core",
        "botclick-ui": "csprclick-ui"
    }
    
    ignore_dirs = {'.git', 'node_modules', 'dist', 'build', '.next', 'artifacts', 'cache'}

    for dirpath, dirnames, filenames in os.walk(root_dir, topdown=False):
        dirnames[:] = [d for d in dirnames if d not in ignore_dirs]
        for filename in filenames:
            old_path = os.path.join(dirpath, filename)
            try:
                with open(old_path, 'r', encoding='utf-8') as f:
                    content = f.read()
                
                new_content = content
                for old, new in replacements.items():
                    new_content = new_content.replace(old, new)
                
                if new_content != content:
                    with open(old_path, 'w', encoding='utf-8') as f:
                        f.write(new_content)
            except:
                pass

if __name__ == "__main__":
    revert_imports("c:\\Users\\OWNER\\Desktop\\Bot prove")
    print("Reverted SDK imports!")
