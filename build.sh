#!/bin/bash
echo "Building frontend..."
cd web
npm run build
cd ..

echo "Copying frontend files..."
rm -rf internal/frontend/dist
mkdir -p internal/frontend/dist
cp -r web/dist/* internal/frontend/dist/

echo "Building Go binary..."
go build -o relationship ./cmd/server

echo "Done!"
echo "Run: ./relationship"