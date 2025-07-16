#!/usr/bin/env python3
"""
Simple test script to verify the Sentinel AI API setup and test the endpoint
"""

import requests
import json
import os
import time

def check_prerequisites():
    """Check if required files exist"""
    print("🔍 Checking prerequisites...")
    
    required_files = ["original.tf", "logs.json", ".env"]
    missing_files = []
    
    for file in required_files:
        if not os.path.exists(file):
            missing_files.append(file)
        else:
            print(f"✅ {file} exists")
    
    if missing_files:
        print(f"❌ Missing files: {missing_files}")
        return False
    
    print("✅ All required files present")
    return True

def test_server_health():
    """Test if the server is running"""
    print("\n🌐 Testing server health...")
    
    try:
        response = requests.get("http://localhost:8000/docs", timeout=5)
        if response.status_code == 200:
            print("✅ Server is running and accessible")
            return True
        else:
            print(f"❌ Server responded with status: {response.status_code}")
            return False
    except requests.exceptions.ConnectionError:
        print("❌ Server is not running. Start it with:")
        print("   cd /home/rishi/Desktop/Sentinel-AI/multi_role_agent")
        print("   source venv/bin/activate")
        print("   python -m uvicorn agent:app --host 0.0.0.0 --port 8000 --reload")
        return False
    except Exception as e:
        print(f"❌ Error connecting to server: {e}")
        return False

def test_process_terraform():
    """Test the main API endpoint"""
    print("\n🚀 Testing /process_terraform/ endpoint...")
    print("⏳ This may take 2-5 minutes...")
    
    start_time = time.time()
    
    try:
        response = requests.post(
            "http://localhost:8000/process_terraform/",
            headers={
                "Content-Type": "application/json",
                "Accept": "application/json"
            },
            timeout=600  # 10 minute timeout for AI processing
        )
        
        elapsed_time = time.time() - start_time
        print(f"⏱️  Processing time: {elapsed_time:.2f} seconds")
        
        if response.status_code == 200:
            result = response.json()
            print("✅ API call successful!")
            
            # Check response structure
            expected_fields = ["report", "original_graph", "changed_graph", "changed_terraform", "original_terraform"]
            for field in expected_fields:
                if field in result:
                    content_length = len(str(result[field]))
                    print(f"✅ {field}: {content_length} characters")
                else:
                    print(f"❌ Missing field: {field}")
            
            # Save results for inspection
            with open("test_report.json", "w") as f:
                json.dump(result, f, indent=2)
            print("💾 Full response saved to test_report.json")
            
            # Save individual components
            if "report" in result:
                with open("test_vulnerability_report.txt", "w") as f:
                    f.write(result["report"])
                print("📊 Vulnerability report saved to test_vulnerability_report.txt")
            
            if "changed_terraform" in result:
                with open("test_fixed_terraform.tf", "w") as f:
                    f.write(result["changed_terraform"])
                print("🔧 Fixed Terraform code saved to test_fixed_terraform.tf")
            
            return True
            
        else:
            print(f"❌ API call failed with status code: {response.status_code}")
            print("Response content:")
            try:
                error_data = response.json()
                print(json.dumps(error_data, indent=2))
            except:
                print(response.text)
            return False
            
    except requests.exceptions.Timeout:
        print("❌ Request timed out. The AI processing is taking longer than expected.")
        print("   This could be due to:")
        print("   - Complex Terraform configuration")
        print("   - Google Gemini API rate limits") 
        print("   - Network issues")
        return False
    except Exception as e:
        print(f"❌ Error testing API: {e}")
        return False

def main():
    """Main test function"""
    print("🧪 Sentinel AI API Test Suite")
    print("=" * 50)
    
    # Check prerequisites
    if not check_prerequisites():
        print("\n❌ Prerequisites not met. Please fix the issues above.")
        return
    
    # Test server health
    if not test_server_health():
        print("\n❌ Server health check failed. Please start the server.")
        return
    
    # Test main endpoint
    success = test_process_terraform()
    
    print("\n" + "=" * 50)
    if success:
        print("🎉 All tests passed! Your API is working correctly.")
        print("\n📋 Next steps:")
        print("1. Check test_vulnerability_report.txt for security analysis")
        print("2. Review test_fixed_terraform.tf for remediated code")
        print("3. Use Postman with the same configuration for interactive testing")
    else:
        print("❌ Tests failed. Please check the error messages above.")

if __name__ == "__main__":
    main()
